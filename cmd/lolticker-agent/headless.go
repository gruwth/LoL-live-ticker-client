package main

import (
	"context"
	"log/slog"

	"lolticker-agent/internal/agent"
)

// runHeadless logs state transitions and nothing else. It is what the nogui
// build always does, and what the GUI build falls back to when there is no
// front end to run.
func runHeadless(ctx context.Context, ag *agent.Agent, log *slog.Logger) {
	states := ag.States()
	var (
		prev     agent.State
		havePrev bool
	)
	transitioned := func(old agent.State, cur agent.State) bool {
		return !havePrev ||
			old.Status != cur.Status ||
			old.RiotID != cur.RiotID ||
			old.ShareURL != cur.ShareURL ||
			old.LastError != cur.LastError
	}
	for {
		select {
		case <-ctx.Done():
			return
		case st := <-states:
			attrs := []any{"status", st.Status}
			if st.RiotID != "" {
				attrs = append(attrs, "riotId", st.RiotID)
			}
			if st.ShareURL != "" {
				attrs = append(attrs, "shareUrl", st.ShareURL)
			}
			if st.LatencyMS > 0 {
				attrs = append(attrs, "latencyMs", st.LatencyMS)
			}
			if st.LastError != "" {
				attrs = append(attrs, "err", st.LastError)
			}
			// Only transitions are worth a line. While live the clock advances
			// every second, and logging that at Info would bury everything
			// else in a wall of identical entries.
			if transitioned(prev, st) {
				log.Info("state", attrs...)
			} else {
				log.Debug("state", append(attrs, "gameTime", st.GameTime)...)
			}
			prev, havePrev = st, true
		}
	}
}
