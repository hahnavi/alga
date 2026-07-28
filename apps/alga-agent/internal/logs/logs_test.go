package logs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeLogFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.log")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunPrintsAll(t *testing.T) {
	path := writeLogFile(t, "line1\nline2\nline3\n")
	var buf bytes.Buffer
	if err := Run(&buf, []string{"--file", path}); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "line1\nline2\nline3\n" {
		t.Errorf("got %q", got)
	}
}

func TestRunTail(t *testing.T) {
	path := writeLogFile(t, "a\nb\nc\nd\ne\n")
	var buf bytes.Buffer
	if err := Run(&buf, []string{"--file", path, "-n", "2"}); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "d\ne\n" {
		t.Errorf("got %q", got)
	}
}

func TestRunTailAlias(t *testing.T) {
	path := writeLogFile(t, "a\nb\nc\n")
	var buf bytes.Buffer
	if err := Run(&buf, []string{"--file", path, "--tail", "1"}); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "c\n" {
		t.Errorf("got %q", got)
	}
}

func TestRunTailMoreThanLines(t *testing.T) {
	path := writeLogFile(t, "x\ny\n")
	var buf bytes.Buffer
	if err := Run(&buf, []string{"--file", path, "-n", "10"}); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "x\ny\n" {
		t.Errorf("got %q", got)
	}
}

func TestRunMissingFile(t *testing.T) {
	err := Run(&bytes.Buffer{}, []string{"--file", "/nonexistent/agent.log"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRunEmptyFile(t *testing.T) {
	path := writeLogFile(t, "")
	var buf bytes.Buffer
	if err := Run(&buf, []string{"--file", path}); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}
