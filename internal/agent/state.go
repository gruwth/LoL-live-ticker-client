package agent

import (
	"math"
	"sync"
	"time"
)

// Status is the one-word answer to "is this thing working", which is the only
// question most users ever have.
type Status string

const (
	StatusOffline Status = "offline" // not connected to relay
	StatusOnline  Status = "online"  // connected, no game
	StatusLive    Status = "live"    // in a game, streaming
	StatusError   Status = "error"   // auth failure, etc.
)

// State is everything a front end needs. It is a plain value with no
// behaviour, so consuming it pulls in nothing from this package.
type State struct {
	Status    Status
	RiotID    string
	ShareURL  string
	GameTime  float64
	LatencyMS int
	LastError string // empty when healthy
	UpdatedAt time.Time
}

// sameAs compares everything that a front end would render. UpdatedAt is
// excluded on purpose: it changes every time and would defeat the whole point.
func (s State) sameAs(o State) bool {
	return s.Status == o.Status &&
		s.RiotID == o.RiotID &&
		s.ShareURL == o.ShareURL &&
		math.Floor(s.GameTime) == math.Floor(o.GameTime) &&
		s.LatencyMS == o.LatencyMS &&
		s.LastError == o.LastError
}

// RelayState is what the network half of the app reports in. It is defined
// here rather than imported from internal/relay so that the core keeps its
// one-way dependencies; main wires the two together.
type RelayState struct {
	Connected bool
	ShareURL  string
	LatencyMS int
	Err       string
}

// states is the publishing side of the state channel. It coalesces: the
// channel holds one value, and a new publish replaces an unread one, so a slow
// front end always gets the newest state rather than a backlog of stale ones.
type states struct {
	mu        sync.Mutex
	ch        chan State
	cur       State
	published State
	havePub   bool

	// The two halves that Status is derived from.
	connected bool
	inGame    bool
}

func newStates() *states {
	return &states{
		ch:  make(chan State, 1),
		cur: State{Status: StatusOffline},
	}
}

// Status returns the current status without consuming the channel. The channel
// has one consumer by design; anything else that needs to react to status,
// like the updater gating on a live game, reads it here instead of competing
// for values.
func (a *Agent) Status() Status {
	a.states.mu.Lock()
	defer a.states.mu.Unlock()
	return a.states.cur.Status
}

// States returns the channel front ends consume. Headless mode logs
// transitions from it; the GUI feeds it to the window and tray.
func (a *Agent) States() <-chan State { return a.states.ch }

// setRelay merges in what the relay reports and publishes if anything changed.
func (s *states) setRelay(rs RelayState) {
	s.mu.Lock()
	s.cur.ShareURL = rs.ShareURL
	s.cur.LatencyMS = rs.LatencyMS
	s.cur.LastError = rs.Err
	s.connected = rs.Connected
	s.recompute()
	s.mu.Unlock()
}

// setGame merges in what the poll loop sees.
func (s *states) setGame(inGame bool, riotID string, gameTime float64) {
	s.mu.Lock()
	s.inGame = inGame
	s.cur.RiotID = riotID
	s.cur.GameTime = gameTime
	s.recompute()
	s.mu.Unlock()
}

// recompute derives Status from the two halves and publishes on change.
// Caller holds the lock.
func (s *states) recompute() {
	switch {
	case s.cur.LastError != "":
		s.cur.Status = StatusError
	case !s.connected:
		s.cur.Status = StatusOffline
	case s.inGame:
		s.cur.Status = StatusLive
	default:
		s.cur.Status = StatusOnline
	}

	// Publish on change only. While live the clock advances every second so
	// this is 1 Hz by nature; idle and offline go quiet, which is the case
	// that matters because it is the one the agent spends most of its life in.
	if s.havePub && s.published.sameAs(s.cur) {
		return
	}
	s.cur.UpdatedAt = time.Now()
	s.published, s.havePub = s.cur, true

	select {
	case s.ch <- s.cur:
	default:
		// Consumer has not read the previous value; replace it.
		select {
		case <-s.ch:
		default:
		}
		select {
		case s.ch <- s.cur:
		default:
		}
	}
}
