// Package logging configures a single structured logger (log/slog, JSON) for
// the agent and provides redaction helpers used by LLM request/response logging.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"

	"alga-agent/internal/config"
)

// Logger is the package-level logger used across the agent after Setup.
var Logger *slog.Logger

// Options configures Setup.
type Options struct {
	// Level is one of debug|info|warn|error (unknown = info).
	Level string
	// File is the log file path. Empty means the default
	// <data dir>/logs/agent.log; the literal "stderr" disables file logging.
	File string
	// MaxSizeMB is the rotation threshold per file (default 5).
	MaxSizeMB int
	// BackupCount is the number of rotated files kept (default 3).
	BackupCount int
}

// FileLoggingDisabled is the Options.File value that turns off file logging.
const FileLoggingDisabled = "stderr"

// DefaultLogFile returns the default log location under the agent data dir.
func DefaultLogFile() string {
	return filepath.Join(config.ResolveDataDir(), "logs", "agent.log")
}

// Setup configures the package-level Logger. Logs always go to stderr (so
// journald/console capture keeps working under the systemd service) and, by
// default, additionally to a size-rotated file (hermes convention: 5 MB x 3
// backups). The returned closer should be deferred by the caller.
func Setup(opts Options) (io.Closer, error) {
	var w io.Writer = os.Stderr
	var closer io.Closer = io.NopCloser(nil)

	file := opts.File
	if file == "" {
		file = DefaultLogFile()
	}
	if file != FileLoggingDisabled {
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return nil, fmt.Errorf("create log dir: %w", err)
		}
		maxSize := opts.MaxSizeMB
		if maxSize <= 0 {
			maxSize = 5
		}
		backups := opts.BackupCount
		if backups < 0 {
			backups = 3
		}
		lj := &lumberjack.Logger{
			Filename:   file,
			MaxSize:    maxSize,
			MaxBackups: backups,
		}
		w = io.MultiWriter(os.Stderr, lj)
		closer = lj
	}

	lvl := ParseLevel(opts.Level)
	Logger = slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl}))
	slog.SetDefault(Logger)
	return closer, nil
}

// ParseLevel converts a level string to slog.Level. Unknown => Info.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Debug, Info, Warn, Error are thin wrappers around the package Logger.
func Debug(msg string, args ...any) { Logger.Debug(msg, args...) }
func Info(msg string, args ...any)  { Logger.Info(msg, args...) }
func Warn(msg string, args ...any)  { Logger.Warn(msg, args...) }
func Error(msg string, args ...any) { Logger.Error(msg, args...) }

// redacted is the placeholder value for sensitive fields in logs.
const redacted = "[REDACTED]"

var sensitiveKeys = map[string]struct{}{
	"api_key":            {},
	"apikey":             {},
	"key":                {},
	"token":              {},
	"bot_token":          {},
	"agent_token":        {},
	"authorization":      {},
	"password":           {},
	"secret":             {},
	"pepper":             {},
	"set-cookie":         {},
	"cookie":             {},
	"openai_api_key":     {},
	"search_api_key":     {},
	"alga_agent_token":   {},
	"telegram_bot_token": {},
}

// IsSensitiveKey reports whether k is a known sensitive key.
func IsSensitiveKey(k string) bool {
	_, ok := sensitiveKeys[strings.ToLower(strings.TrimSpace(k))]
	return ok
}

// RedactedString returns the redaction placeholder when value is non-empty, so
// that sensitive strings are never logged wholesale.
func RedactedString(value string) string {
	if value == "" {
		return ""
	}
	return redacted
}

// redactionMu guards the redacting writer during concurrent appends.
var redactionMu sync.Mutex

// RedactingWriter wraps a writer and replaces sensitive substrings in the
// written bytes with the redaction placeholder. It is intended for coarse,
// best-effort redaction of HTTP request/response dumps used in debug logging;
// structured logging with explicit RedactedString calls is always preferred.
type RedactingWriter struct {
	out     io.Writer
	needles []string
}

// NewRedactingWriter returns a writer that replaces each of needles with the
// redaction placeholder before forwarding to out.
func NewRedactingWriter(out io.Writer, needles []string) *RedactingWriter {
	return &RedactingWriter{out: out, needles: needles}
}

func (r *RedactingWriter) Write(p []byte) (int, error) {
	redactionMu.Lock()
	defer redactionMu.Unlock()
	s := string(p)
	for _, n := range r.needles {
		if n != "" {
			s = strings.ReplaceAll(s, n, redacted)
		}
	}
	_, err := r.out.Write([]byte(s))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
