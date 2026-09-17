// Package agent is the state machine: poll the game, transform, hand to relay.
package agent

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	"lolticker-agent/internal/riot"
	"lolticker-agent/internal/transform"
	"lolticker-agent/internal/wire"
)

const (
	tick          = time.Second
	idlePoll      = 5 * time.Second
	idleHeartbeat = 30 * time.Second
	forceSnapshot = 30 * time.Second
	// Three failed ticks in a row while in game means the client died without
	// sending GameEnd.
	missesForEnd = 3
)

// Sender is the relay, narrowed to what the loop needs.
type Sender interface {
	Send(typ string, data any)
}

type Agent struct {
	riot     *riot.Client
	out      Sender
	activeMu sync.RWMutex
	active   bool // include the local player's gold/stats/abilities
	states   *states
	log      *slog.Logger
	nowFunc  func() time.Time
}

func New(rc *riot.Client, out Sender, includeActive bool, log *slog.Logger) *Agent {
	return &Agent{
		riot:    rc,
		out:     out,
		active:  includeActive,
		states:  newStates(),
		log:     log,
		nowFunc: time.Now,
	}
}

// SetShareActivePlayer applies the privacy toggle without a restart. It is
// read once per tick, so the next snapshot already reflects it.
func (a *Agent) SetShareActivePlayer(v bool) {
	a.activeMu.Lock()
	a.active = v
	a.activeMu.Unlock()
}

func (a *Agent) shareActive() bool {
	a.activeMu.RLock()
	defer a.activeMu.RUnlock()
	return a.active
}

// SetRelayState is how the network half reports in. main wires this to the
// relay client's status callback, which keeps internal/relay out of this
// package's imports and this package out of the UI's.
func (a *Agent) SetRelayState(rs RelayState) { a.states.setRelay(rs) }

type state int

const (
	stateIdle state = iota
	stateInGame
)

func (a *Agent) Run(ctx context.Context) error {
	t := time.NewTicker(tick)
	defer t.Stop()

	var (
		st           = stateIdle
		lastPoll     time.Time
		lastIdle     time.Time
		lastSnapshot time.Time
		lastHash     uint64
		lastSeenEvt  = transform.NoEventsSeen
		misses       int
	)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
		now := a.nowFunc()

		// While idle the game is polled every 5 s, not every tick.
		if st == stateIdle && now.Sub(lastPoll) < idlePoll {
			a.heartbeat(now, &lastIdle)
			continue
		}
		lastPoll = now

		data, err := a.riot.AllGameData(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if st == stateIdle {
				if !riot.IsGameNotRunning(err) {
					a.log.Debug("poll failed while idle", "err", err)
				}
				a.heartbeat(now, &lastIdle)
				continue
			}
			// In game: tolerate the odd timeout, but a client that is really
			// gone ends the game.
			if riot.IsGameNotRunning(err) {
				misses++
				a.log.Debug("game not answering", "misses", misses)
				if misses >= missesForEnd {
					a.log.Info("game ended (client gone)")
					a.out.Send(wire.TypeGameEnd, wire.GameEnd{})
					st, misses, lastSeenEvt, lastHash = stateIdle, 0, transform.NoEventsSeen, 0
					a.states.setGame(false, "", 0)
					lastIdle = time.Time{}
				}
				continue
			}
			a.log.Warn("poll failed", "err", err)
			continue
		}
		misses = 0

		if st == stateIdle {
			a.log.Info("game found", "mode", data.GameData.GameMode, "map", data.GameData.MapName)
			st = stateInGame
			lastSeenEvt = transform.NoEventsSeen
			lastHash = 0
			lastSnapshot = time.Time{}
		}

		// Snapshot first, then the events that produced it, so a client that
		// renders the feed sees the state the events refer to.
		snap := transform.Snapshot(data, a.shareActive())
		// RiotID comes from the raw payload, not the snapshot: the user may
		// have opted out of sharing active-player data, but the front end
		// still needs to show whose game this is.
		a.states.setGame(true, data.ActivePlayer.RiotID, snap.Game.Time)

		if b, err := json.Marshal(snap); err != nil {
			a.log.Error("marshal snapshot", "err", err)
		} else {
			h := hash(b)
			if h != lastHash || now.Sub(lastSnapshot) >= forceSnapshot {
				a.out.Send(wire.TypeSnapshot, snap)
				lastHash, lastSnapshot = h, now
			}
		}

		newEvents, next := transform.Events(data.Events.Events, lastSeenEvt)
		lastSeenEvt = next

		if len(newEvents) > 0 {
			a.out.Send(wire.TypeEvents, newEvents)
		}

		if end, ok := gameEnd(newEvents); ok {
			a.log.Info("game ended", "result", end.Result)
			a.out.Send(wire.TypeGameEnd, end)
			st, lastSeenEvt, lastHash = stateIdle, transform.NoEventsSeen, 0
			a.states.setGame(false, "", 0)
			lastIdle = time.Time{}
		}
	}
}

// heartbeat tells the site "online, not in game" every 30 s. A zero lastIdle
// means the agent just dropped back to idle, so one goes out right away.
func (a *Agent) heartbeat(now time.Time, lastIdle *time.Time) {
	if now.Sub(*lastIdle) < idleHeartbeat {
		return
	}
	a.out.Send(wire.TypeIdle, nil)
	*lastIdle = now
}

func gameEnd(events []wire.Event) (wire.GameEnd, bool) {
	for _, e := range events {
		if e.Name == "GameEnd" {
			return wire.GameEnd{Result: e.Result}, true
		}
	}
	return wire.GameEnd{}, false
}

func hash(b []byte) uint64 {
	h := fnv.New64a()
	h.Write(b)
	return h.Sum64()
}
