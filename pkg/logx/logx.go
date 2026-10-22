// Package logx builds the service logger and carries the request ID in context.
package logx

import (
	"context"
	"io"
	"log/slog"
	"os"
)

type ctxKey struct{}

// New returns a JSON slog.Logger writing to stdout at the given level
// ("debug", "info", "warn" or "error", case-insensitive). An empty or
// unrecognized level falls back to info and logs a warning saying so.
func New(level string) *slog.Logger {
	return newLogger(os.Stdout, level)
}

func newLogger(w io.Writer, level string) *slog.Logger {
	var lvl slog.Level
	err := lvl.UnmarshalText([]byte(level))
	if err != nil {
		lvl = slog.LevelInfo
	}
	l := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl}))
	if err != nil {
		l.Warn("unrecognized log level, using info", "requested_level", level)
	}
	return l
}

// WithRequestID returns a copy of ctx carrying the request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestID returns the request ID stored in ctx, or "" if there is none.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}
