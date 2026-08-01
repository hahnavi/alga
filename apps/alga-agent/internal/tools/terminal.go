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
	"time"

	"alga-agent/internal/config"
)

const (
	terminalMarker   = "___ALGA_EXIT_%d___"
	terminalMarkerRe = "___ALGA_EXIT_"
	defaultMaxOutput = 64 * 1024
	defaultTimeout   = 120 * time.Second
	defaultCWD       = ""
)

// TerminalTool provides a persistent shell session for the agent. Unlike the
// old whitelisted ShellTool, this runs arbitrary commands in a long-lived bash
// process, maintaining working directory and environment across calls. It is
// designed for deep SRE investigation: kubectl, systemctl, journalctl, curl,
// grep across logs, etc.
type TerminalTool struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	reader    *bufio.Reader
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
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr,omitempty"`
	ExitCode int    `json:"exit_code"`
	CWD      string `json:"cwd"`
	TimedOut bool   `json:"timed_out,omitempty"`
	Duration string `json:"duration_ms"`
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
	}
}

func (t *TerminalTool) ensureStarted() error {
	if t.started {
		return nil
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	if _, err := os.Stat(shell); err != nil {
		shell = "/bin/sh"
	}

	t.cmd = exec.Command(shell, "-i")
	t.cmd.Env = append(os.Environ(), "TERM=dumb", "NO_COLOR=1")
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
	t.cmd.Stderr = t.cmd.Stdout

	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("terminal: start shell: %w", err)
	}
	t.stdin = stdin
	t.reader = bufio.NewReaderSize(stdout, 256*1024)
	t.started = true
	return nil
}

func (t *TerminalTool) execute(ctx context.Context, in terminalInput) Result[terminalOutput] {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.ensureStarted(); err != nil {
		return Err[terminalOutput](err)
	}

	if in.CWD != "" {
		t.cwd = in.CWD
	}

	timeout := t.timeout
	if in.Timeout > 0 {
		req := time.Duration(in.Timeout) * time.Second
		if req < timeout {
			timeout = req
		}
	}

	marker := fmt.Sprintf(terminalMarker, 0)
	script := in.Command
	if t.cwd != "" {
		script = fmt.Sprintf("cd %s 2>/dev/null; %s", shellQuote(t.cwd), script)
	}
	full := fmt.Sprintf("%s; echo \"%s\" $?\n", script, terminalMarkerRe)

	start := time.Now()
	if _, err := io.WriteString(t.stdin, full); err != nil {
		t.reset()
		return Err[terminalOutput](fmt.Errorf("terminal: write: %w", err))
	}

	deadline := time.After(timeout)
	var out strings.Builder
	exitCode := -1

	for {
		select {
		case <-deadline:
			t.kill()
			return OK(terminalOutput{
				Stdout:   truncate(out.String(), t.maxOutput),
				ExitCode: -1,
				CWD:      t.cwd,
				TimedOut: true,
				Duration: fmt.Sprintf("%d", time.Since(start).Milliseconds()),
			})
		case <-ctx.Done():
			t.kill()
			return Err[terminalOutput](ctx.Err())
		default:
		}

		line, err := t.reader.ReadString('\n')
		out.WriteString(line)

		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, terminalMarkerRe) {
			fmt.Sscanf(trimmed, terminalMarkerRe+"%d", &exitCode)
			break
		}
		if err != nil {
			t.reset()
			return Err[terminalOutput](fmt.Errorf("terminal: read: %w", err))
		}
	}

	output := out.String()
	if idx := strings.LastIndex(output, terminalMarkerRe); idx >= 0 {
		output = output[:idx]
	}

	_ = marker
	return OK(terminalOutput{
		Stdout:   truncate(strings.TrimRight(output, "\n"), t.maxOutput),
		ExitCode: exitCode,
		CWD:      t.cwd,
		Duration: fmt.Sprintf("%d", time.Since(start).Milliseconds()),
	})
}

func (t *TerminalTool) kill() {
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	t.reset()
}

func (t *TerminalTool) reset() {
	t.started = false
	t.cmd = nil
	t.stdin = nil
	t.reader = nil
}

// Close terminates the persistent shell session.
func (t *TerminalTool) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stdin != nil {
		_ = t.stdin.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_ = t.cmd.Wait()
	}
	t.reset()
	return nil
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\n'\"\\$&|;()<>") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated]..."
}

// RegisterTerminalTool registers the terminal tool if non-nil.
func RegisterTerminalTool(reg *Registry, t *TerminalTool) {
	if t == nil {
		return
	}
	reg.Register(NewTypedTool(
		"terminal",
		"Execute a shell command in a persistent terminal session. Supports arbitrary commands, pipes, redirects, and chaining. Working directory and environment persist across calls. Use for system investigation: kubectl, systemctl, journalctl, curl, grep, etc.",
		t.execute,
		WithCategory[terminalInput, terminalOutput]("System"),
	))
}
