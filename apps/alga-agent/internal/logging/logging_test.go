package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupDefaultFileUnderDataDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ALGA_AGENT_HOME", home)

	closer, err := Setup(Options{Level: "info"})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer closer.Close()

	Info("hello from test", "k", "v")

	path := filepath.Join(home, "logs", "agent.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("default log file missing: %v", err)
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "{") || !strings.Contains(line, `"hello from test"`) {
		t.Fatalf("expected JSON log line, got %q", line)
	}
}

func TestSetupStderrOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ALGA_AGENT_HOME", home)

	closer, err := Setup(Options{Level: "info", File: FileLoggingDisabled})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer closer.Close()

	if _, err := os.Stat(filepath.Join(home, "logs")); !os.IsNotExist(err) {
		t.Fatalf("logs dir should not exist, stat err: %v", err)
	}
}

func TestSetupExplicitFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "custom.log")

	closer, err := Setup(Options{Level: "debug", File: path})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	Debug("dbg msg")
	if err := closer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("log file missing: %v", err)
	}
	if !strings.Contains(string(data), "dbg msg") {
		t.Fatalf("expected debug line in file, got %q", string(data))
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]string{
		"debug":   "DEBUG",
		"info":    "INFO",
		"warn":    "WARN",
		"warning": "WARN",
		"error":   "ERROR",
		"bogus":   "INFO",
		"":        "INFO",
	}
	for in, want := range cases {
		if got := ParseLevel(in).String(); got != want {
			t.Errorf("ParseLevel(%q) = %s, want %s", in, got, want)
		}
	}
}
