package agent

import (
	"context"
	"log/slog"
	"time"

	"lolticker-agent/internal/lcu"
	"lolticker-agent/internal/transform"
	"lolticker-agent/internal/wire"
)

const (
	phasePoll       = 5 * time.Second // one short string; the client is up far more than the game is
	champSelectPoll = time.Second     // only while actually in champ select
)

// LCUWatch reports client phase and champ select. It is constructed only when
// the user has turned the feature on, so an agent with it off performs no
// process scan and makes no request — there is nothing here to accidentally
// run.
type LCUWatch struct {
	lcu *lcu.Client
	out Sender
	log *slog.Logger

	lastPhase string
	lastCS    string
}

func NewLCUWatch(out Sender, log *slog.Logger) *LCUWatch {
	return &LCUWatch{lcu: lcu.New(log), out: out, log: log}
}

// Run polls until ctx is done.
func (w *LCUWatch) Run(ctx context.Context) {
	w.log.Info("league client integration is on",
		"endpoints", []string{lcu.PathGameflowPhase, lcu.PathChampSelect})

	t := time.NewTicker(champSelectPoll)
	defer t.Stop()

	var lastPhasePoll time.Time
	inChampSelect := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}

		now := time.Now()
		if now.Sub(lastPhasePoll) >= phasePoll {
			lastPhasePoll = now
			inChampSelect = w.pollPhase(ctx) == "ChampSelect"
		}

		// The session endpoint is only touched while champ select is actually
		// happening. Outside it there is nothing to read.
		if inChampSelect {
			w.pollChampSelect(ctx)
		}
	}
}

func (w *LCUWatch) pollPhase(ctx context.Context) string {
	raw, err := w.lcu.GameflowPhase(ctx)
	if err != nil {
		if !lcu.IsNotRunning(err) {
			w.log.Debug("phase poll failed", "err", err)
		}
		// A closed client is "none" as far as anyone watching is concerned.
		w.emitPhase(wire.Phase{Phase: "none"})
		return ""
	}

	w.emitPhase(transform.Phase(raw))
	return raw
}

// emitPhase sends on change only, not every poll.
func (w *LCUWatch) emitPhase(p wire.Phase) {
	if p.Phase == w.lastPhase {
		return
	}
	w.lastPhase = p.Phase
	w.log.Info("client phase", "phase", p.Phase)
	w.out.Send(wire.TypePhase, p)
}

func (w *LCUWatch) pollChampSelect(ctx context.Context) {
	s, err := w.lcu.ChampSelect(ctx)
	if err != nil {
		if !lcu.IsNotRunning(err) && err != lcu.ErrNoChampSelect {
			w.log.Debug("champ select poll failed", "err", err)
		}
		return
	}
	// Spectating someone else's champ select is not the user's own game.
	if s.IsSpectating {
		return
	}

	cs := transform.ChampSelect(s, "")
	// Champ select is about ninety seconds, so this is a handful of messages
	// per lock rather than a stream.
	key := csKey(cs)
	if key == w.lastCS {
		return
	}
	w.lastCS = key
	w.out.Send(wire.TypeChampSelect, cs)
}

// csKey is a cheap change check over everything that can actually move.
func csKey(cs wire.ChampSelect) string {
	b := make([]byte, 0, 128)
	for _, id := range cs.Bans {
		b = appendInt(b, id)
	}
	b = append(b, '|')
	for _, p := range append(append([]wire.CSPlayer{}, cs.MyTeam...), cs.TheirTeam...) {
		b = appendInt(b, p.ChampionID)
	}
	return string(b)
}

func appendInt(b []byte, n int) []byte {
	if n == 0 {
		return append(b, '0', ',')
	}
	var tmp [8]byte
	i := len(tmp)
	for n > 0 {
		i--
		tmp[i] = byte('0' + n%10)
		n /= 10
	}
	return append(append(b, tmp[i:]...), ',')
}

// ResetChampSelect clears the change tracking so the next poll re-sends. Used
// on reconnect, where the relay has lost everything.
func (w *LCUWatch) ResetChampSelect() { w.lastCS, w.lastPhase = "", "" }
