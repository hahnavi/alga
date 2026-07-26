// Package service manages running alga-agent as a systemd user service on
// Linux. It generates a unit file under the user's systemd directory, wires
// the standard install/uninstall/start/stop/restart/status verbs to
// `systemctl --user`, and enables lingering so the service survives logout.
// The design is a Go-idiomatic port of hermes-agent's gateway service
// management, trimmed to user scope and a single instance.
package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"alga-agent/internal/config"
)

const (
	// ServiceName is the systemd unit base name.
	ServiceName = "alga-agent"
	unitName    = ServiceName + ".service"
)

// runCmd executes a command and returns its combined output. It is a
// package-level variable so tests can stub out systemctl/loginctl calls.
var runCmd = func(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// execPath returns the running binary's resolved path. Stubbed in tests.
var execPath = func() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, rerr := filepath.EvalSymlinks(p); rerr == nil {
		resolved = filepath.Clean(resolved)
		return resolved, nil
	}
	return filepath.Clean(p), nil
}

// UnitPath returns the user-scope unit file location:
// $XDG_CONFIG_HOME/systemd/user/alga-agent.service, defaulting XDG_CONFIG_HOME
// to ~/.config.
func UnitPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			home = "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "systemd", "user", unitName)
}

// GenerateUnit renders the systemd unit for the given binary and data
// directory. Restart policy mirrors hermes-agent: always restart with a short
// backoff and no start-limit window, logging to the journal.
func GenerateUnit(binPath, dataDir string) string {
	return fmt.Sprintf(`[Unit]
Description=Alga Agent - AIOps AI assistant
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
Type=simple
ExecStart=%s
WorkingDirectory=%s
Environment="ALGA_AGENT_HOME=%s"
Restart=always
RestartSec=5
KillSignal=SIGTERM
TimeoutStopSec=20
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
`, quoteExecStart(binPath), dataDir, dataDir)
}

// quoteExecStart wraps a binary path in double quotes when it contains
// whitespace so systemd parses it as a single argv element. Paths without
// whitespace are emitted verbatim. WorkingDirectory is a single-value
// directive (not argv-split) so it needs no quoting.
func quoteExecStart(p string) string {
	if strings.ContainsAny(p, " \t") {
		return fmt.Sprintf("%q", p)
	}
	return p
}

// checkSystemd verifies we are on Linux with a reachable per-user systemd
// instance before touching unit files.
func checkSystemd() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemd service management is only supported on Linux (detected %s)", runtime.GOOS)
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemctl not found in PATH — is this a systemd-based system?")
	}
	if out, err := runCmd("systemctl", "--user", "is-system-running"); err != nil {
		// is-system-running exits non-zero for degraded states that are still
		// usable; only treat a failed bus connection as fatal.
		if strings.Contains(out, "Failed to connect") || strings.Contains(out, "No such file or directory") {
			return fmt.Errorf("cannot reach the systemd user instance: %s\n"+
				"Hints: ensure you are logged in as this user (XDG_RUNTIME_DIR must be set),\n"+
				"and enable lingering with: loginctl enable-linger %s", out, currentUsername())
		}
	}
	return nil
}

func currentUsername() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return os.Getenv("USER")
}

// isTempPath reports whether p lives under a temp directory. Installing a
// unit pointing at a `go run`/`go test` scratch binary would break on reboot.
func isTempPath(p string) bool {
	tmp := filepath.Clean(os.TempDir()) + string(os.PathSeparator)
	return strings.HasPrefix(p, tmp) || strings.HasPrefix(p, "/tmp/")
}

// resolveInstallPaths returns the binary path and data dir used in the unit.
func resolveInstallPaths() (bin, dataDir string, err error) {
	bin, err = execPath()
	if err != nil {
		return "", "", fmt.Errorf("resolve executable path: %w", err)
	}
	if isTempPath(bin) {
		return "", "", fmt.Errorf("refusing to install a unit pointing at temporary binary %s — "+
			"build alga-agent to a stable location (e.g. go build -o ~/.local/bin/alga-agent) and re-run", bin)
	}
	return bin, config.ResolveDataDir(), nil
}

// InstallOptions controls Install behavior.
type InstallOptions struct {
	Force  bool // overwrite an existing, different unit file
	Enable bool // systemctl --user enable (start on login)
	Now    bool // systemctl --user start after install
}

// Install writes the unit file, reloads systemd, optionally enables/starts
// the service, and enables lingering so it keeps running after logout.
func Install(w io.Writer, opts InstallOptions) error {
	if err := checkSystemd(); err != nil {
		return err
	}
	bin, dataDir, err := resolveInstallPaths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir %s: %w", dataDir, err)
	}

	unit := GenerateUnit(bin, dataDir)
	path := UnitPath()
	if existing, rerr := os.ReadFile(path); rerr == nil && string(existing) != unit && !opts.Force {
		return fmt.Errorf("unit file %s already exists with different content — re-run with --force to overwrite", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create systemd user dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("write unit file %s: %w", path, err)
	}
	fmt.Fprintf(w, "Wrote %s\n", path)

	if out, err := runCmd("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl --user daemon-reload: %v: %s", err, out)
	}
	if opts.Enable {
		if out, err := runCmd("systemctl", "--user", "enable", unitName); err != nil {
			return fmt.Errorf("systemctl --user enable: %v: %s", err, out)
		}
		fmt.Fprintln(w, "Enabled service (starts on login).")
	}
	if opts.Now {
		if out, err := runCmd("systemctl", "--user", "start", unitName); err != nil {
			return fmt.Errorf("systemctl --user start: %v: %s", err, out)
		}
		fmt.Fprintln(w, "Service started.")
	}

	ensureLinger(w)
	fmt.Fprintf(w, "Logs: journalctl --user -u %s -f\n", ServiceName)
	return nil
}

// ensureLinger enables lingering so the user service keeps running after
// logout. Best-effort: on failure it prints the manual command instead.
func ensureLinger(w io.Writer) {
	name := currentUsername()
	if lingerEnabled(name) {
		return
	}
	if out, err := runCmd("loginctl", "enable-linger", name); err != nil {
		fmt.Fprintf(w, "Warning: could not enable lingering (%v: %s).\n"+
			"Without it the service stops at logout. Enable manually with:\n"+
			"  sudo loginctl enable-linger %s\n", err, out, name)
		return
	}
	fmt.Fprintln(w, "Enabled lingering (service keeps running after logout).")
}

func lingerEnabled(name string) bool {
	out, err := runCmd("loginctl", "show-user", name, "--property=Linger")
	return err == nil && strings.TrimSpace(strings.TrimPrefix(out, "Linger=")) == "yes"
}

// Uninstall stops and disables the service, removes the unit file, and
// reloads systemd. Stop/disable failures are non-fatal (the service may not
// be running or enabled).
func Uninstall(w io.Writer) error {
	if err := checkSystemd(); err != nil {
		return err
	}
	if out, err := runCmd("systemctl", "--user", "stop", unitName); err != nil && out != "" {
		fmt.Fprintf(w, "Note: stop: %s\n", out)
	}
	if out, err := runCmd("systemctl", "--user", "disable", unitName); err != nil && out != "" {
		fmt.Fprintf(w, "Note: disable: %s\n", out)
	}
	path := UnitPath()
	if err := os.Remove(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove unit file %s: %w", path, err)
		}
		fmt.Fprintf(w, "Unit file %s was not installed.\n", path)
	} else {
		fmt.Fprintf(w, "Removed %s\n", path)
	}
	if out, err := runCmd("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl --user daemon-reload: %v: %s", err, out)
	}
	fmt.Fprintln(w, "Service uninstalled.")
	return nil
}

// refreshUnitIfStale regenerates the unit and rewrites it (plus
// daemon-reload) when the installed file no longer matches — e.g. after the
// binary moved or the data dir changed. Missing unit files are left alone;
// Start will surface the real systemd error.
func refreshUnitIfStale(w io.Writer) {
	existing, err := os.ReadFile(UnitPath())
	if err != nil {
		return
	}
	bin, dataDir, err := resolveInstallPaths()
	if err != nil {
		return
	}
	unit := GenerateUnit(bin, dataDir)
	if string(existing) == unit {
		return
	}
	if err := os.WriteFile(UnitPath(), []byte(unit), 0o644); err != nil {
		fmt.Fprintf(w, "Warning: could not refresh stale unit file: %v\n", err)
		return
	}
	if out, rerr := runCmd("systemctl", "--user", "daemon-reload"); rerr != nil {
		fmt.Fprintf(w, "Warning: daemon-reload after unit refresh failed: %s\n", out)
		return
	}
	fmt.Fprintln(w, "Refreshed stale unit file.")
}

// Start starts the service, refreshing a stale unit file first.
func Start(w io.Writer) error {
	if err := checkSystemd(); err != nil {
		return err
	}
	refreshUnitIfStale(w)
	if out, err := runCmd("systemctl", "--user", "start", unitName); err != nil {
		return fmt.Errorf("systemctl --user start: %v: %s", err, out)
	}
	fmt.Fprintln(w, "Service started.")
	return nil
}

// Stop stops the service.
func Stop(w io.Writer) error {
	if err := checkSystemd(); err != nil {
		return err
	}
	if out, err := runCmd("systemctl", "--user", "stop", unitName); err != nil {
		return fmt.Errorf("systemctl --user stop: %v: %s", err, out)
	}
	fmt.Fprintln(w, "Service stopped.")
	return nil
}

// Restart restarts the service, refreshing a stale unit file first.
func Restart(w io.Writer) error {
	if err := checkSystemd(); err != nil {
		return err
	}
	refreshUnitIfStale(w)
	if out, err := runCmd("systemctl", "--user", "restart", unitName); err != nil {
		return fmt.Errorf("systemctl --user restart: %v: %s", err, out)
	}
	fmt.Fprintln(w, "Service restarted.")
	return nil
}

// Status prints the service state, linger state, and a journal hint. An
// inactive service is informational, not an error.
func Status(w io.Writer) error {
	if err := checkSystemd(); err != nil {
		return err
	}
	active, _ := runCmd("systemctl", "--user", "is-active", unitName)
	fmt.Fprintf(w, "%s: %s\n", ServiceName, orUnknown(active))

	if out, _ := runCmd("systemctl", "--user", "--no-pager", "status", unitName); out != "" {
		fmt.Fprintln(w, out)
	}

	name := currentUsername()
	if lingerEnabled(name) {
		fmt.Fprintln(w, "Lingering: enabled (service survives logout).")
	} else {
		fmt.Fprintf(w, "Lingering: disabled — the service stops at logout. Enable with: loginctl enable-linger %s\n", name)
	}
	fmt.Fprintf(w, "Logs: journalctl --user -u %s -f\n", ServiceName)
	return nil
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
