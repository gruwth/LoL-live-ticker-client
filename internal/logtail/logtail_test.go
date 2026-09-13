package logtail

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

func newTestHandler(max int) (*Handler, *slog.Logger) {
	h := New(slog.NewTextHandler(io.Discard, nil), max)
	return h, slog.New(h)
}

func TestKeepsTheMostRecentLines(t *testing.T) {
	h, log := newTestHandler(3)
	for i := 0; i < 10; i++ {
		log.Info("line", "n", i)
	}
	lines := h.Lines()
	if len(lines) != 3 {
		t.Fatalf("kept %d lines, want 3", len(lines))
	}
	if !strings.Contains(lines[0], "n=7") || !strings.Contains(lines[2], "n=9") {
		t.Errorf("buffer holds the wrong window: %v", lines)
	}
}

// A logger built with slog.With must still be captured, or half the app's
// output would silently never reach the window.
func TestDerivedLoggersAreStillCaptured(t *testing.T) {
	h, log := newTestHandler(10)
	log.With("component", "relay").Info("connected")
	log.WithGroup("g").Info("grouped")

	text := h.Text()
	if !strings.Contains(text, "connected") {
		t.Errorf("WithAttrs logger was not captured: %q", text)
	}
	if !strings.Contains(text, "grouped") {
		t.Errorf("WithGroup logger was not captured: %q", text)
	}
}

// Pasting a log into a chat must not hand over the user's credentials.
func TestTokenIsRedacted(t *testing.T) {
	h, log := newTestHandler(10)
	const token = "lt_abcdefghijklmnop"
	h.Redact(token)

	log.Info("starting", "token", token)
	log.Error("the relay rejected " + token)

	text := h.Text()
	if strings.Contains(text, token) {
		t.Fatalf("the token survived redaction: %q", text)
	}
	if !strings.Contains(text, Redacted) {
		t.Errorf("nothing was redacted: %q", text)
	}
}

// Redacting something tiny would blank out half the log.
func TestShortSecretsAreNotRedacted(t *testing.T) {
	h, log := newTestHandler(10)
	h.Redact("ab")
	log.Info("a stable connection")
	if strings.Contains(h.Text(), Redacted) {
		t.Errorf("a two-character secret was redacted: %q", h.Text())
	}
}

func TestLinesIsACopy(t *testing.T) {
	h, log := newTestHandler(5)
	log.Info("one")
	lines := h.Lines()
	lines[0] = "tampered"
	if strings.Contains(h.Text(), "tampered") {
		t.Error("Lines handed out the internal slice")
	}
}

func TestConcurrentWritesAndReads(t *testing.T) {
	h, log := newTestHandler(50)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 500; i++ {
			log.Info("write", "n", i)
		}
		close(done)
	}()
	for i := 0; i < 500; i++ {
		_ = h.Text()
	}
	<-done
}
