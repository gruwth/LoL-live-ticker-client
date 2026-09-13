//go:build nogui

package main

import "context"

// runFrontend for the headless build. Built with -tags nogui this binary is
// pure Go and cgo-free, pulls in no display libraries, and is the smallest
// auditable form of the agent.
//
// The single-instance lock is still served, so a second launch exits quietly
// instead of fighting the first one over the relay connection. There is no
// window to show.
func runFrontend(ctx context.Context, deps frontendDeps) {
	go deps.lock.Serve(func() {
		deps.log.Info("another instance tried to start; this one is already running")
	})
	runHeadless(ctx, deps.agent, deps.log)
}

// guiBuild tells main whether a window exists to ask the user things.
const guiBuild = false
