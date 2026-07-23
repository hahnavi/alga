package mcp

import (
	"os"
	"os/exec"
)

// These helpers are in a separate file so they can be stubbed out in tests
// without polluting the main client.go.

func writeFileImpl(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func runGoCmd(dir string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &goCmdError{cmd: "go " + joinArgs(args), out: string(out), err: err}
	}
	return nil
}

func execLookPath(name string) (string, error) {
	return exec.LookPath(name)
}

type goCmdError struct {
	cmd string
	out string
	err error
}

func (e *goCmdError) Error() string {
	return "go cmd failed: " + e.cmd + ": " + e.err.Error() + "\n" + e.out
}

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}
