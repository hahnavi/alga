package service

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isLinux reports whether the systemd code paths can run: checkSystemd
// requires linux and a real systemctl in PATH (LookPath is not stubbed).
func isLinux() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := exec.LookPath("systemctl")
	return err == nil
}

// stubRunner replaces runCmd, recording invocations and returning canned
// responses keyed by the joined command line.
type stubRunner struct {
	calls     []string
	responses map[string]struct {
		out string
		err error
	}
}

func (s *stubRunner) run(name string, args ...string) (string, error) {
	call := name + " " + strings.Join(args, " ")
	s.calls = append(s.calls, call)
	if r, ok := s.responses[call]; ok {
		return r.out, r.err
	}
	return "", nil
}

func withStubRunner(t *testing.T) *stubRunner {
	t.Helper()
	s := &stubRunner{responses: map[string]struct {
		out string
		err error
	}{}}
	orig := runCmd
	runCmd = s.run
	t.Cleanup(func() { runCmd = orig })
	return s
}

func withStubExec(t *testing.T, path string) {
	t.Helper()
	orig := execPath
	execPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { execPath = orig })
}

// fakeBin is a stable (non-temp) binary path; Install never stats it.
const fakeBin = "/usr/local/bin/alga-agent"

func TestGenerateUnit(t *testing.T) {
	unit := GenerateUnit("/usr/local/bin/alga-agent", "/home/u/.alga")

	for _, want := range []string{
		"ExecStart=/usr/local/bin/alga-agent\n",
		"WorkingDirectory=/home/u/.alga\n",
		`Environment="ALGA_AGENT_HOME=/home/u/.alga"` + "\n",
		"Restart=always\n",
		"RestartSec=5\n",
		"StartLimitIntervalSec=0\n",
		"WantedBy=default.target\n",
		"StandardOutput=journal\n",
		"KillSignal=SIGTERM\n",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit missing %q:\n%s", want, unit)
		}
	}
}

func TestGenerateUnit_QuotedSpacedPath(t *testing.T) {
	unit := GenerateUnit("/home/my user/apps/alga-agent", "/home/my user/.alga")
	// ExecStart must quote the spaced binary so systemd treats it as one argv.
	if !strings.Contains(unit, `ExecStart="/home/my user/apps/alga-agent"`+"\n") {
		t.Errorf("ExecStart not quoted for spaced path:\n%s", unit)
	}
	// A no-space path stays unquoted.
	plain := GenerateUnit("/usr/local/bin/alga-agent", "/home/u/.alga")
	if !strings.Contains(plain, "ExecStart=/usr/local/bin/alga-agent\n") {
		t.Errorf("no-space path should stay unquoted:\n%s", plain)
	}
}

func TestUnitPathRespectsXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	want := filepath.Join(dir, "systemd", "user", "alga-agent.service")
	if got := UnitPath(); got != want {
		t.Errorf("UnitPath() = %q, want %q", got, want)
	}
}

func TestUnitPathDefaultsToHomeConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := filepath.Join(home, ".config", "systemd", "user", "alga-agent.service")
	if got := UnitPath(); got != want {
		t.Errorf("UnitPath() = %q, want %q", got, want)
	}
}

func TestInstallRefusesTempBinary(t *testing.T) {
	if !isLinux() {
		t.Skip("systemd install path is linux-only")
	}
	withStubRunner(t)
	withStubExec(t, filepath.Join(os.TempDir(), "go-build123", "alga-agent"))

	var buf bytes.Buffer
	err := Install(&buf, InstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "temporary binary") {
		t.Fatalf("Install() err = %v, want temp-binary refusal", err)
	}
}

func TestInstallWritesUnitAndRunsSystemctl(t *testing.T) {
	if !isLinux() {
		t.Skip("systemd install path is linux-only")
	}
	s := withStubRunner(t)
	// Pretend linger is already enabled so no enable-linger call happens.
	s.responses["loginctl show-user "+currentUsername()+" --property=Linger"] = struct {
		out string
		err error
	}{out: "Linger=yes"}

	bin := fakeBin
	withStubExec(t, bin)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("ALGA_AGENT_HOME", t.TempDir())

	var buf bytes.Buffer
	if err := Install(&buf, InstallOptions{Enable: true, Now: true}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(xdg, "systemd", "user", "alga-agent.service"))
	if err != nil {
		t.Fatalf("unit file not written: %v", err)
	}
	if !strings.Contains(string(data), "ExecStart="+bin) {
		t.Errorf("unit ExecStart wrong:\n%s", data)
	}

	joined := strings.Join(s.calls, "\n")
	for _, want := range []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable alga-agent.service",
		"systemctl --user start alga-agent.service",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing call %q, got:\n%s", want, joined)
		}
	}
}

func TestInstallRefusesDifferentExistingUnitWithoutForce(t *testing.T) {
	if !isLinux() {
		t.Skip("systemd install path is linux-only")
	}
	withStubRunner(t)
	withStubExec(t, fakeBin)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("ALGA_AGENT_HOME", t.TempDir())

	unitPath := filepath.Join(xdg, "systemd", "user", "alga-agent.service")
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitPath, []byte("[Unit]\nDescription=other\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := Install(&buf, InstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("Install() err = %v, want existing-unit refusal", err)
	}

	if err := Install(&buf, InstallOptions{Force: true}); err != nil {
		t.Fatalf("Install(force) error: %v", err)
	}
}

func TestUninstallRemovesUnitAndReloads(t *testing.T) {
	if !isLinux() {
		t.Skip("systemd path is linux-only")
	}
	s := withStubRunner(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	unitPath := filepath.Join(xdg, "systemd", "user", "alga-agent.service")
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := Uninstall(&buf); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Errorf("unit file still exists")
	}

	joined := strings.Join(s.calls, "\n")
	for _, want := range []string{
		"systemctl --user stop alga-agent.service",
		"systemctl --user disable alga-agent.service",
		"systemctl --user daemon-reload",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing call %q, got:\n%s", want, joined)
		}
	}
}

func TestIsTempPath(t *testing.T) {
	if !isTempPath("/tmp/go-build/alga-agent") {
		t.Error("expected /tmp path to be temp")
	}
	if isTempPath("/usr/local/bin/alga-agent") {
		t.Error("expected /usr/local/bin not to be temp")
	}
}
