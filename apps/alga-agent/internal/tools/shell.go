package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"alga-agent/internal/config"
)

// ShellTool executes whitelisted shell commands. It is NOT a true sandbox:
// commands run with the agent process privileges. Restrict allowed_commands
// and run the binary under a least-privilege user/container (SPEC §6.2).
type ShellTool struct {
	allowed    map[string]struct{}
	maxOutput  int
	defaultTTL time.Duration
}

// NewShellTool constructs a ShellTool from config. Returns nil if disabled.
func NewShellTool(cfg config.ShellConfig) *ShellTool {
	if !cfg.Enabled {
		return nil
	}
	return &ShellTool{
		allowed:    cfg.AllowedCommandSet(),
		maxOutput:  cfg.MaxOutputBytes,
		defaultTTL: cfg.Timeout,
	}
}

// Name implements Tool.
func (s *ShellTool) Name() string { return "shell" }

// Description implements Tool.
func (s *ShellTool) Description() string {
	return "Execute a shell command from the allowed list (" + strings.Join(s.allowedList(), ", ") + ") and return stdout, stderr, and exit code. Not a true sandbox — commands run with the agent's privileges."
}

// Schema implements Tool.
func (s *ShellTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The command to execute. Must be in the allowed list; arguments follow after a space.",
			},
			"args": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional explicit arguments (preferred over embedding in command).",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Per-call timeout in seconds (must not exceed configured max).",
			},
		},
		"required": []string{"command"},
	}
}

// Execute implements Tool.
func (s *ShellTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Command        string   `json:"command"`
		Args           []string `json:"args"`
		TimeoutSeconds int      `json:"timeout_seconds"`
	}
	if err := DecodeArgs(args, &in); err != nil {
		return "", err
	}
	if in.Command == "" {
		return "", errors.New("command is required")
	}

	// Parse command into binary + remainder, supporting either a single command
	// string with spaces or an explicit args array.
	binary, inlineArgs := splitCommand(in.Command)
	cmdArgs := inlineArgs
	if len(in.Args) > 0 {
		cmdArgs = in.Args
	}

	if _, ok := s.allowed[binary]; !ok {
		return "", fmt.Errorf("command %q is not in the allowed list", binary)
	}

	// Per-call timeout: use the smaller of requested and configured default.
	ttl := s.defaultTTL
	if in.TimeoutSeconds > 0 {
		req := time.Duration(in.TimeoutSeconds) * time.Second
		if req < ttl {
			ttl = req
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, ttl)
	defer cancel()

	cmd := exec.CommandContext(execCtx, binary, cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	// Context.DeadlineExceeded surfaces as a non-zero exit; report explicitly.
	timedOut := execCtx.Err() == context.DeadlineExceeded

	out := stdout.Bytes()
	if len(out) > s.maxOutput {
		out = append(out[:s.maxOutput], []byte("\n...[truncated]...")...)
	}
	errOut := stderr.Bytes()
	if len(errOut) > s.maxOutput/2 {
		errOut = append(errOut[:s.maxOutput/2], []byte("\n...[truncated]...")...)
	}

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			// Non-exit error (e.g. binary not found, context cancellation).
			return "", fmt.Errorf("command execution error: %w", err)
		}
	}

	result := map[string]any{
		"command":     binary + " " + strings.Join(cmdArgs, " "),
		"exit_code":   exitCode,
		"stdout":      string(out),
		"stderr":      string(errOut),
		"duration_ms": elapsed.Milliseconds(),
	}
	if timedOut {
		result["timed_out"] = true
	}
	return JSONString(result), nil
}

func (s *ShellTool) allowedList() []string {
	out := make([]string, 0, len(s.allowed))
	for k := range s.allowed {
		out = append(out, k)
	}
	return out
}

// splitCommand splits a command string into the binary name and the remainder.
// It honors simple quoting for paths with spaces (best-effort, not a full shell
// parser — callers should use the explicit args array for complex cases).
func splitCommand(cmd string) (string, []string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "", nil
	}
	// Simple split on spaces; the agent is instructed to use the args array
	// for arguments containing spaces.
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}

// RegisterShellTool registers the shell tool if non-nil.
func RegisterShellTool(reg *Registry, t *ShellTool) {
	if t == nil {
		return
	}
	reg.Register(t)
}
