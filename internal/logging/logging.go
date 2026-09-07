// Package logging configures the process-wide structured logger (L5).
//
// All operational logging goes through log/slog. Set LOG_FORMAT=json for
// machine-readable output (default is human "text"), and LOG_LEVEL to one of
// debug|info|warn|error (default info).
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type ctxKey struct{ name string }

// RequestIDKey is the context key under which the HTTP layer stores a per-request
// correlation id. contextHandler promotes it to a log attribute automatically.
var RequestIDKey = ctxKey{"request_id"}

// WithRequestID returns a child context carrying id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, RequestIDKey, id)
}

// RequestIDFromContext returns the correlation id, or "".
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(RequestIDKey).(string); ok {
		return v
	}
	return ""
}

// Init installs the default slog logger. Safe to call once at startup.
func Init(format, level string) {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var base slog.Handler
	if strings.EqualFold(strings.TrimSpace(format), "json") {
		base = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		base = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(&contextHandler{base}))
}

// contextHandler adds the request-id attribute to any record whose context
// carries one, so handlers can just call slog.InfoContext(ctx, ...).
type contextHandler struct{ slog.Handler }

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestIDFromContext(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{h.Handler.WithGroup(name)}
}
