//go:build !nogui

package main

import (
	"context"
	"log/slog"

	"lolticker-agent/internal/agent"
	"lolticker-agent/internal/singleton"
)

// runFrontend for the desktop build.
//
// The window and tray are not wired in yet — that is the next step of the v3
// scope, and it is the point at which this file starts importing Fyne and the
// build starts requiring cgo and OpenGL. Until then this behaves exactly like
// the nogui build, so the seam is real but the dependency is not yet taken on.
func runFrontend(ctx context.Context, lock *singleton.Lock, ag *agent.Agent, log *slog.Logger) {
	go lock.Serve(func() {
		log.Info("another instance asked for the window")
	})
	runHeadless(ctx, ag, log)
}
