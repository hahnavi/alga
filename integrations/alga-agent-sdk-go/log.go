package alga

import (
	"fmt"
	"log/slog"
)

// defaultUserAgent is sent on every outbound request so the backend can
// identify SDK traffic. Override per-client via WithUserAgent.
const defaultUserAgent = "alga-agent-sdk-go"

// Logf is retained for backward compatibility with callers that replaced it
// to integrate with their own logger. New code should pass a structured
// Logger via WithLogger instead. The default routes through slog.Default at
// Debug level.
//
// Deprecated: use WithLogger.
var Logf = func(format string, args ...any) {
	slog.Default().Debug(fmt.Sprintf(format, args...))
}

// slogLogger adapts a *slog.Logger to the Logger interface.
type slogLogger struct{ l *slog.Logger }

// AsLogger wraps any *slog.Logger so it can be passed to WithLogger.
func AsLogger(l *slog.Logger) Logger {
	if l == nil {
		l = slog.Default()
	}
	return &slogLogger{l: l}
}

func (s *slogLogger) Debug(msg string, args ...any) { s.l.Debug(msg, args...) }
func (s *slogLogger) Info(msg string, args ...any)  { s.l.Info(msg, args...) }
func (s *slogLogger) Warn(msg string, args ...any)  { s.l.Warn(msg, args...) }
func (s *slogLogger) Error(msg string, args ...any) { s.l.Error(msg, args...) }
