package tools

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"alga-agent/internal/config"
	"alga-agent/internal/logging"
)

const (
	terminalMarkerRe = "___ALGA_EXIT_"
	defaultMaxOutput = 64 * 1024
	defaultTimeout   = 120 * time.Second
	defaultCWD       = ""
)

// TerminalTool provides a persistent shell session for the agent. Unlike the
// old whitelisted ShellTool, this runs arbitrary commands in a long-lived POSIX
// shell process, maintaining environment across calls. It is designed for deep
// SRE investigation: kubectl, systemctl, journalctl, curl, grep across logs,
// etc. The shell runs in its own process group so commands and their children
// can be killed together, and output is read in a goroutine so timeouts and
// context cancellation stay responsive even while a command blocks.
type TerminalTool struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	outLines  chan string
	quit      chan struct{}
	stderrBuf *strings.Builder
	stderrMu  sync.Mutex
	cwd       string
	maxOutput int
	timeout   time.Duration
	started   bool
}

type terminalInput struct {
	Command string `json:"command" desc:"The shell command to execute. Supports pipes, redirects, and chaining (&&, ||, ;)."`
	Timeout int    `json:"timeout_seconds,omitempty" desc:"Per-call timeout in seconds. Defaults to the configured max."`
	CWD     string `json:"cwd,omitempty" desc:"Working directory for this command. Persists for subsequent calls."`
}

type terminalOutput struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr,omitempty"`
	ExitCode   int    `json:"exit_code"`
	CWD        string `json:"cwd"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// NewTerminalTool constructs a TerminalTool from config. Returns nil if disabled.
func NewTerminalTool(cfg config.TerminalConfig) *TerminalTool {
	if !cfg.Enabled {
		return nil
	}
	maxOut := cfg.MaxOutputBytes
	if maxOut <= 0 {
		maxOut = defaultMaxOutput
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	cwd := cfg.CWD
	if cwd == "" {
		cwd = defaultCWD
	}
	return &TerminalTool{
		cwd:       cwd,
		maxOutput: maxOut,
		timeout:   timeout,
		stderrBuf: &strings.Builder{},
	}
}

func (t *TerminalTool) ensureStarted() error {
	if t.started {
		return nil
	}
	// Always use a fixed non-interactive POSIX shell. Honoring $SHELL or running
	// interactively (-i) would let the environment inject an arbitrary shell and
	// produce prompt noise on the captured output.
	const shell = "/bin/sh"
	if _, err := os.Stat(shell); err != nil {
		return fmt.Errorf("terminal: POSIX shell %s not available: %w", shell, err)
	}

	t.cmd = exec.Command(shell)
	t.cmd.Env = append(os.Environ(), "TERM=dumb", "NO_COLOR=1")
	// Run the shell in its own process group so kill can signal the shell and
	// any child processes it spawned together.
	t.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if t.cwd != "" {
		t.cmd.Dir = t.cwd
	}

	stdin, err := t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("terminal: stdin pipe: %w", err)
	}
	stdout, err := t.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("terminal: stdout pipe: %w", err)
	}
	stderr, err := t.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("terminal: stderr pipe: %w", err)
	}

	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("terminal: start shell: %w", err)
	}

	t.stdin = stdin
	t.quit = make(chan struct{})
	t.outLines = make(chan string, 64)
	go readLines(bufio.NewReaderSize(stdout, 256*1024), t.outLines, t.quit)
	go drainStderr(bufio.NewReaderSize(stderr, 64*1024), t.stderrBuf, &t.stderrMu)
	t.started = true
	return nil
}

func (t *TerminalTool) execute(ctx context.Context, in terminalInput) (result Result[terminalOutput]) {
	start := time.Now()
	defer func() {
		status := "ok"
		if !result.OK {
			status = "error"
		}
		auditTerminalCommand(ctx, in, status, result.Error, time.Since(start))
	}()

	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.ensureStarted(); err != nil {
		return Err[terminalOutput](err)
	}

	// Validate the requested working directory before persisting it so a failed
	// change returns an error and the command is not run in the wrong place.
	if in.CWD != "" {
		info, err := os.Stat(in.CWD)
		if err != nil {
			return Err[terminalOutput](fmt.Errorf("terminal: invalid cwd %q: %w", in.CWD, err))
		}
		if !info.IsDir() {
			return Err[terminalOutput](fmt.Errorf("terminal: cwd %q is not a directory", in.CWD))
		}
		t.cwd = in.CWD
	}

	timeout := t.timeout
	if in.Timeout > 0 {
		req := time.Duration(in.Timeout) * time.Second
		if req < timeout {
			timeout = req
		}
	}

	t.stderrMu.Lock()
	t.stderrBuf.Reset()
	t.stderrMu.Unlock()

	script := in.Command
	if t.cwd != "" {
		script = fmt.Sprintf("cd %s; %s", shellQuote(t.cwd), script)
	}
	full := fmt.Sprintf("%s; echo \"%s\" $?\n", script, terminalMarkerRe)

	if _, err := io.WriteString(t.stdin, full); err != nil {
		t.kill()
		return Err[terminalOutput](fmt.Errorf("terminal: write: %w", err))
	}

	deadline := time.After(timeout)
	var out strings.Builder
	exitCode := -1
	timedOut := false

readLoop:
	for {
		select {
		case <-deadline:
			timedOut = true
			t.kill()
			break readLoop
		case <-ctx.Done():
			t.kill()
			return Err[terminalOutput](ctx.Err())
		case line, ok := <-t.outLines:
			if !ok {
				t.reset()
				return Err[terminalOutput](fmt.Errorf("terminal: shell exited unexpectedly"))
			}
			// Always inspect the line for the sentinel so the marker is detected
			// even after output accumulation has been bounded.
			if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, terminalMarkerRe) {
				fmt.Sscanf(trimmed, terminalMarkerRe+"%d", &exitCode)
				break readLoop
			}
			if out.Len() < t.maxOutput {
				out.WriteString(line)
			}
		}
	}

	t.stderrMu.Lock()
	stderrText := t.stderrBuf.String()
	t.stderrMu.Unlock()

	output := out.String()
	if idx := strings.LastIndex(output, terminalMarkerRe); idx >= 0 {
		output = output[:idx]
	}
	output = strings.TrimRight(output, "\n")

	return OK(terminalOutput{
		Stdout:     truncate(output, t.maxOutput),
		Stderr:     truncate(stderrText, t.maxOutput),
		ExitCode:   exitCodeOrTimeout(exitCode, timedOut),
		CWD:        t.cwd,
		TimedOut:   timedOut,
		DurationMS: time.Since(start).Milliseconds(),
	})
}

func exitCodeOrTimeout(code int, timedOut bool) int {
	if timedOut {
		return -1
	}
	return code
}

// signalGroup signals the shell's entire process group (the shell plus any
// children it spawned). It falls back to signaling only the shell if the group
// signal fails.
func (t *TerminalTool) signalGroup(sig syscall.Signal) {
	if t.cmd == nil || t.cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-t.cmd.Process.Pid, sig); err != nil {
		_ = t.cmd.Process.Signal(sig)
	}
}

// stopLocked kills the shell process group, closes stdin, reaps the shell, and
// resets session state. Callers must hold t.mu. Safe to call when not started.
func (t *TerminalTool) stopLocked() {
	t.signalGroup(syscall.SIGKILL)
	if t.stdin != nil {
		_ = t.stdin.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Wait()
	}
	t.reset()
}

func (t *TerminalTool) kill() {
	t.stopLocked()
}

func (t *TerminalTool) reset() {
	if t.quit != nil {
		close(t.quit)
	}
	t.started = false
	t.cmd = nil
	t.stdin = nil
	t.outLines = nil
	t.quit = nil
}

// Close terminates the persistent shell session.
func (t *TerminalTool) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopLocked()
	return nil
}

// readLines streams lines from r into ch until EOF/error, at which point it
// closes ch. It selects on quit so it can exit promptly after a kill/reset
// without blocking on a channel send that no one is draining.
func readLines(r *bufio.Reader, ch chan string, quit <-chan struct{}) {
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			select {
			case ch <- line:
			case <-quit:
				return
			}
		}
		if err != nil {
			close(ch)
			return
		}
	}
}

// drainStderr accumulates stderr lines into buf (guarded by mu) until EOF. The
// persistent shell interleaves stderr asynchronously, so attribution to a single
// command is best-effort: execute resets buf before each command and snapshots it
// after.
func drainStderr(r *bufio.Reader, buf *strings.Builder, mu *sync.Mutex) {
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			mu.Lock()
			buf.WriteString(line)
			mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// auditTerminalCommand emits one structured audit event per terminal command,
// covering both successful and failed invocations.
func auditTerminalCommand(ctx context.Context, in terminalInput, status, errMsg string, elapsed time.Duration) {
	if logging.Logger == nil {
		return
	}
	var chatID, sessionID string
	if cc, ok := CallContextFrom(ctx); ok {
		chatID, sessionID = cc.ChatID, cc.SessionID
	}
	args := []any{
		"tool", "terminal",
		"command", in.Command,
		"cwd", in.CWD,
		"chat_id", chatID,
		"session_id", sessionID,
		"status", status,
		"elapsed_ms", elapsed.Milliseconds(),
	}
	if errMsg != "" {
		args = append(args, "err", errMsg)
	}
	if status == "ok" {
		logging.Info("terminal command", args...)
	} else {
		logging.Warn("terminal command", args...)
	}
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	trunc := s[:limit]
	// Back off to a valid UTF-8 rune boundary so the JSON result stays valid.
	for len(trunc) > 0 && !utf8.ValidString(trunc) {
		trunc = trunc[:len(trunc)-1]
	}
	return trunc + "\n...[truncated]..."
}

// RegisterTerminalTool registers the terminal tool if non-nil.
func RegisterTerminalTool(reg *Registry, t *TerminalTool) {
	if t == nil {
		return
	}
	reg.Register(NewTypedTool(
		"terminal",
		"Execute a shell command in a persistent terminal session. Supports arbitrary commands, pipes, redirects, and chaining. Environment persists across calls; the working directory is set via the cwd parameter and persists until changed. Use for system investigation: kubectl, systemctl, journalctl, curl, grep, etc.",
		t.execute,
		WithCategory[terminalInput, terminalOutput]("System"),
	))
}
