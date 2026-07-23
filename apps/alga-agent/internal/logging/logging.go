// Package logging configures a single structured logger (log/slog, JSON) for
// the agent and provides redaction helpers used by LLM request/response logging.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Logger is the package-level logger used across the agent after Setup.
var Logger *slog.Logger

// Setup configures the package-level Logger from level and file path.
// level is one of debug|info|warn|error. file is optional ("" = stderr).
// If file is set, logs go to that file (opened append-only). The returned
// closer should be deferred by the caller.
func Setup(level, file string) (io.Closer, error) {
	var w io.Writer = os.Stderr
	var closer io.Closer = io.NopCloser(nil)

	if file != "" {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil, err
		}
		w = f
		closer = f
	}

	lvl := ParseLevel(level)
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
