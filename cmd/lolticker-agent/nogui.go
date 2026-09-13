//go:build nogui

package main

import (
	"context"
	"log/slog"

	"lolticker-agent/internal/agent"
	"lolticker-agent/internal/singleton"
)

// runFrontend for the headless build. Built with -tags nogui this binary is
// pure Go and cgo-free, pulls in no display libraries, and is the smallest
// auditable form of the agent.
//
// The single-instance lock is still served, so a second launch exits quietly
// instead of fighting the first one over the relay connection. There is no
// window to show.
func runFrontend(ctx context.Context, lock *singleton.Lock, ag *agent.Agent, log *slog.Logger) {
	go lock.Serve(func() {
		log.Info("another instance tried to start; this one is already running")
	})
	runHeadless(ctx, ag, log)
}
