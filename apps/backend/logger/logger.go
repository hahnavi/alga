package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
)

var (
	// globalLogger is swapped by Init; atomic so logging from one goroutine
	// never races a concurrent re-init (e.g. parallel tests calling Init).
	globalLogger atomic.Pointer[slog.Logger]
	globalLevel  = new(slog.LevelVar)

	fileMu  sync.Mutex
	logFile *os.File
)

func Init(levelStr string, logFile string) {
	InitWithFormat(levelStr, "", logFile)
}

func InitWithFormat(levelStr string, format string, logFilePath string) {
	fileMu.Lock()
	defer fileMu.Unlock()

	if logFile != nil {
		if err := logFile.Close(); err != nil {
			slog.Warn("failed to close previous log file", "error", err)
		}
		logFile = nil
	}

	var w io.Writer = os.Stdout
	if logFilePath != "" {
		if err := os.MkdirAll(filepath.Dir(logFilePath), 0755); err != nil {
			slog.Error("failed to create log directory", "path", filepath.Dir(logFilePath), "error", err)
			os.Exit(1)
		}
		f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			slog.Error("failed to open log file", "path", logFilePath, "error", err)
			os.Exit(1)
		}
		logFile = f
		w = io.MultiWriter(os.Stdout, f)
	}

	globalLevel.Set(parseLevel(levelStr))

	opts := &slog.HandlerOptions{
		Level:     globalLevel,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				if src, ok := a.Value.Any().(*slog.Source); ok {
					a.Value = slog.StringValue(filepath.Base(src.File) + ":" + strconv.Itoa(src.Line))
				}
			}
			return a
		},
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	// Wrap with contextHandler so request_id/trace_id/span_id/user_id carried in
	// the request context (or the active OpenTelemetry span) are auto-injected
	// into every log line (W8).
	handler = &contextHandler{inner: handler}

	l := slog.New(handler)
	globalLogger.Store(l)
	slog.SetDefault(l)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func Close() {
	fileMu.Lock()
	defer fileMu.Unlock()
	if logFile != nil {
		if err := logFile.Close(); err != nil {
			slog.Warn("failed to close log file", "error", err)
		}
		logFile = nil
	}
}

func With(args ...any) *slog.Logger {
	if l := globalLogger.Load(); l != nil {
		return l.With(args...)
	}
	return slog.Default()
}

func Debug(msg string, args ...any) {
	if l := globalLogger.Load(); l != nil {
		l.Debug(msg, args...)
	}
}

func DebugCtx(ctx context.Context, msg string, args ...any) {
	if l := globalLogger.Load(); l != nil {
		l.DebugContext(ctx, msg, args...)
	}
}

func Info(msg string, args ...any) {
	if l := globalLogger.Load(); l != nil {
		l.Info(msg, args...)
	}
}

func InfoCtx(ctx context.Context, msg string, args ...any) {
	if l := globalLogger.Load(); l != nil {
		l.InfoContext(ctx, msg, args...)
	}
}

func Warn(msg string, args ...any) {
	if l := globalLogger.Load(); l != nil {
		l.Warn(msg, args...)
	}
}

func WarnCtx(ctx context.Context, msg string, args ...any) {
	if l := globalLogger.Load(); l != nil {
		l.WarnContext(ctx, msg, args...)
	}
}

func Error(msg string, args ...any) {
	if l := globalLogger.Load(); l != nil {
		l.Error(msg, args...)
	}
}

func ErrorCtx(ctx context.Context, msg string, args ...any) {
	if l := globalLogger.Load(); l != nil {
		l.ErrorContext(ctx, msg, args...)
	}
}

func Fatal(msg string, args ...any) {
	if l := globalLogger.Load(); l != nil {
		l.Error(msg, args...)
	} else {
		fmt.Fprintf(os.Stderr, "FATAL: "+msg+"\n", args...)
	}
	os.Exit(1)
}
