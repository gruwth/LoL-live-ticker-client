// Package logtail keeps the last N log lines in memory so the window can show
// them. This is the support tool: when someone says "it doesn't work", asking
// them to paste the log is the difference between a five-minute fix and a
// thread of guesses.
package logtail

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
)

// DefaultLines is how much history the window keeps.
const DefaultLines = 200

// Redacted replaces any occurrence of the token in a log line.
const Redacted = "[redacted]"

// Handler wraps another slog.Handler and remembers what went through it.
type Handler struct {
	inner slog.Handler

	mu     sync.Mutex
	lines  []string
	max    int
	secret string
}

// New wraps inner, keeping at most max lines.
func New(inner slog.Handler, max int) *Handler {
	if max <= 0 {
		max = DefaultLines
	}
	return &Handler{inner: inner, max: max, lines: make([]string, 0, max)}
}

// Redact registers a value that must never appear in the buffer. The token is
// the obvious one: a user pasting their log into a chat should not be handing
// over their credentials at the same time.
func (h *Handler) Redact(secret string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Very short values would redact half the log, so ignore them.
	if len(secret) >= 8 {
		h.secret = secret
	}
}

// Lines returns a copy of the buffer, oldest first.
func (h *Handler) Lines() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.lines))
	copy(out, h.lines)
	return out
}

// Text returns the buffer as one blob, ready for a copy button.
func (h *Handler) Text() string { return strings.Join(h.Lines(), "\n") }

func (h *Handler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	h.append(format(r))
	return h.inner.Handle(ctx, r)
}

func (h *Handler) WithAttrs(as []slog.Attr) slog.Handler {
	return &wrapped{parent: h, inner: h.inner.WithAttrs(as)}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &wrapped{parent: h, inner: h.inner.WithGroup(name)}
}

func (h *Handler) append(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.secret != "" {
		line = strings.ReplaceAll(line, h.secret, Redacted)
	}
	if len(h.lines) == h.max {
		copy(h.lines, h.lines[1:])
		h.lines[len(h.lines)-1] = line
		return
	}
	h.lines = append(h.lines, line)
}

// wrapped keeps derived handlers writing into the same buffer. Without it, a
// logger built with slog.With would silently stop being captured.
type wrapped struct {
	parent *Handler
	inner  slog.Handler
}

func (w *wrapped) Enabled(ctx context.Context, l slog.Level) bool { return w.inner.Enabled(ctx, l) }

func (w *wrapped) Handle(ctx context.Context, r slog.Record) error {
	w.parent.append(format(r))
	return w.inner.Handle(ctx, r)
}

func (w *wrapped) WithAttrs(as []slog.Attr) slog.Handler {
	return &wrapped{parent: w.parent, inner: w.inner.WithAttrs(as)}
}

func (w *wrapped) WithGroup(name string) slog.Handler {
	return &wrapped{parent: w.parent, inner: w.inner.WithGroup(name)}
}

// format renders a record the way the window should show it: short, one line,
// no timestamp noise beyond the clock.
func format(r slog.Record) string {
	var b bytes.Buffer
	b.WriteString(r.Time.Format("15:04:05"))
	b.WriteByte(' ')
	b.WriteString(r.Level.String())
	b.WriteByte(' ')
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		b.WriteByte(' ')
		b.WriteString(a.Key)
		b.WriteByte('=')
		b.WriteString(a.Value.String())
		return true
	})
	return b.String()
}
