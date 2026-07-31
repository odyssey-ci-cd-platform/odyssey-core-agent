package common

import (
	"context"
	"log/slog"
)

type loggerKey struct{}

// ContextWithLogger returns a child context with the given logger
// attached. Use LoggerFromContext to retrieve it downstream.
func ContextWithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// LoggerFromContext returns the logger attached to ctx, or fallback if
// none was set.
func LoggerFromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return fallback
}
