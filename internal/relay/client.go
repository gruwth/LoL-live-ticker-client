// Package relay is the only thing in this program that talks to the network.
// It opens one WebSocket to the configured server and writes wire envelopes.
package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"lolticker-agent/internal/wire"
)

// ErrUnauthorized is permanent: a wrong token must not retry forever.
var ErrUnauthorized = errors.New("relay rejected the token")

// Status is what this package reports to whoever is watching. It carries no
// reference to the agent or any UI; main adapts it.
type Status struct {
	Connected bool
	ShareURL  string
	LatencyMS int
	Err       string
}

const (
	pingEvery   = 20 * time.Second
	pongTimeout = 10 * time.Second
	backoffMin  = time.Second
	backoffMax  = 30 * time.Second
	maxCtrl     = 16
)

type Client struct {
	url     string
	token   string
	version string
	log     *slog.Logger

	onStatus func(Status)
	statusMu sync.Mutex
	status   Status

	mu       sync.Mutex
	snapshot *wire.Envelope // latest only - stale snapshots are worthless
	last     *wire.Envelope // last snapshot handed over, replayed on reconnect
	events   []wire.Event   // all unsent events - these must not be lost
	ctrl     []wire.Envelope
	wake     chan struct{}
}

func New(url, token, version string, log *slog.Logger) *Client {
	return &Client{
		url:     url,
		token:   token,
		version: version,
		log:     log,
		wake:    make(chan struct{}, 1),
	}
}

// OnStatus registers the callback that receives connection changes. It must
// be set before Run and is called from the client's own goroutines.
func (c *Client) OnStatus(fn func(Status)) { c.onStatus = fn }

// mutate applies a change to the status and hands a copy to the callback.
// Status is touched from the session, read and ping goroutines, so it has its
// own lock; the callback runs outside that lock so a front end that calls back
// into this client cannot deadlock.
func (c *Client) mutate(fn func(*Status)) {
	c.statusMu.Lock()
	fn(&c.status)
	snap := c.status
	c.statusMu.Unlock()
	if c.onStatus != nil {
		c.onStatus(snap)
	}
}

// setConnected updates connectivity. Latency is cleared on disconnect so the
// UI cannot show a stale number as if it were current.
func (c *Client) setConnected(up bool) {
	c.mutate(func(s *Status) {
		s.Connected = up
		if up {
			// A reconnect means whatever went wrong before is over. An
			// unauthorized failure never reaches here: it exits instead.
			s.Err = ""
		} else {
			s.LatencyMS = 0
		}
	})
}

// Send queues one message. It never blocks: while disconnected only the newest
// snapshot is kept, events accumulate, and control messages are capped.
func (c *Client) Send(typ string, data any) {
	// Events are queued as values so a reconnect can merge everything that
	// piled up into one message.
	if evs, ok := data.([]wire.Event); ok && typ == wire.TypeEvents {
		c.mu.Lock()
		c.events = append(c.events, evs...)
		c.mu.Unlock()
		c.notify()
		return
	}

	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			c.log.Error("marshal outgoing message", "type", typ, "err", err)
			return
		}
		raw = b
	}
	env := wire.Envelope{V: wire.Version, Type: typ, Data: raw}

	c.mu.Lock()
	if typ == wire.TypeSnapshot {
		c.snapshot = &env
		c.last = &env
	} else if len(c.ctrl) < maxCtrl {
		c.ctrl = append(c.ctrl, env)
	}
	c.mu.Unlock()
	c.notify()
}

func (c *Client) notify() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// Run keeps one connection alive until ctx is cancelled or the token is
// rejected. It returns nil on a clean shutdown.
func (c *Client) Run(ctx context.Context) error {
	backoff := backoffMin
	for {
		connected, err := c.session(ctx)
		switch {
		case ctx.Err() != nil:
			return nil
		case errors.Is(err, ErrUnauthorized):
			return err
		}
		if connected {
			// The server is reachable, so the next outage starts over at 1s.
			backoff = backoffMin
		}
		c.log.Warn("relay disconnected", "err", err, "retry_in", backoff.Round(time.Millisecond))

		wait := backoff/2 + time.Duration(rand.Int63n(int64(backoff/2)+1))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		if backoff *= 2; backoff > backoffMax {
			backoff = backoffMax
		}
	}
}

// session runs one connection: dial, then write until something breaks.
// connected reports whether the dial succeeded, which is what resets backoff.
func (c *Client) session(ctx context.Context) (connected bool, err error) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(dialCtx, c.url, &websocket.DialOptions{ //nolint:bodyclose
		HTTPHeader: http.Header{
			"Authorization": {"Bearer " + c.token},
			"User-Agent":    {"lolticker-agent/" + c.version},
		},
	})
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			c.mutate(func(s *Status) { s.Err = "the relay rejected this token" })
			return false, fmt.Errorf("%w: http %d", ErrUnauthorized, resp.StatusCode)
		}
		return false, fmt.Errorf("dial %s: %w", c.url, err)
	}
	defer func() {
		conn.CloseNow()
		c.setConnected(false)
	}()
	c.log.Info("relay connected", "url", c.url)
	c.setConnected(true)

	connCtx, stop := context.WithCancelCause(ctx)
	defer stop(nil)

	go c.readLoop(connCtx, conn, stop)
	go c.pingLoop(connCtx, conn, stop)

	// seq restarts at 1 on every connection; the relay uses it to detect gaps.
	var seq int64
	// A reconnect starts with a full snapshot, then the events from the gap.
	c.replaySnapshot()

	for {
		env, ok := c.next()
		if !ok {
			select {
			case <-connCtx.Done():
				if ctx.Err() != nil {
					conn.Close(websocket.StatusNormalClosure, "")
					return true, nil
				}
				return true, context.Cause(connCtx)
			case <-c.wake:
			}
			continue
		}
		seq++
		env.Seq = seq
		env.TS = time.Now().UnixMilli()

		writeCtx, wcancel := context.WithTimeout(connCtx, 5*time.Second)
		err := wsjson.Write(writeCtx, conn, env)
		wcancel()
		if err != nil {
			c.unsend(env)
			if cause := context.Cause(connCtx); cause != nil && !errors.Is(cause, context.Canceled) {
				return true, cause
			}
			if ctx.Err() != nil {
				conn.Close(websocket.StatusNormalClosure, "")
				return true, nil
			}
			return true, fmt.Errorf("write: %w", err)
		}
	}
}

// next pops the next message to write: snapshot first, then all pending events
// as one message, then control messages.
func (c *Client) next() (wire.Envelope, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.snapshot != nil {
		env := *c.snapshot
		c.snapshot = nil
		return env, true
	}
	if len(c.events) > 0 {
		b, err := json.Marshal(c.events)
		c.events = nil
		if err == nil {
			return wire.Envelope{V: wire.Version, Type: wire.TypeEvents, Data: b}, true
		}
	}
	if len(c.ctrl) > 0 {
		env := c.ctrl[0]
		c.ctrl = c.ctrl[1:]
		return env, true
	}
	return wire.Envelope{}, false
}

// unsend puts a failed write back so the next connection picks it up.
func (c *Client) unsend(env wire.Envelope) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch env.Type {
	case wire.TypeSnapshot:
		if c.snapshot == nil {
			c.snapshot = &env
		}
	case wire.TypeEvents:
		var evs []wire.Event
		if err := json.Unmarshal(env.Data, &evs); err == nil {
			c.events = append(evs, c.events...)
		}
	default:
		if len(c.ctrl) < maxCtrl {
			c.ctrl = append([]wire.Envelope{env}, c.ctrl...)
		}
	}
}

// replaySnapshot makes sure a fresh connection starts with a full snapshot
// rather than waiting for the next state change in the game.
func (c *Client) replaySnapshot() {
	c.mu.Lock()
	if c.snapshot == nil && c.last != nil {
		c.snapshot = c.last
	}
	c.mu.Unlock()
	c.notify()
}

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn, stop context.CancelCauseFunc) {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			stop(fmt.Errorf("read: %w", err))
			return
		}
		var msg wire.ServerMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.log.Warn("unparsable relay message", "err", err)
			continue
		}
		switch msg.Type {
		case "ok":
			c.log.Debug("relay ok")
			if msg.ShareURL != "" {
				c.mutate(func(s *Status) { s.ShareURL = msg.ShareURL })
			}
		case "error":
			c.log.Error("relay error", "code", msg.Code, "msg", msg.Msg)
			c.mutate(func(s *Status) {
				if s.Err = msg.Msg; s.Err == "" {
					s.Err = msg.Code
				}
			})
			if msg.Code == "unauthorized" {
				stop(fmt.Errorf("%w: %s", ErrUnauthorized, msg.Msg))
				conn.Close(websocket.StatusPolicyViolation, "unauthorized")
				return
			}
		}
	}
}

func (c *Client) pingLoop(ctx context.Context, conn *websocket.Conn, stop context.CancelCauseFunc) {
	t := time.NewTicker(pingEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, pongTimeout)
			sent := time.Now()
			err := conn.Ping(pctx)
			cancel()
			if err == nil {
				ms := int(time.Since(sent).Milliseconds())
				c.mutate(func(s *Status) { s.LatencyMS = ms })
			}
			if err != nil && ctx.Err() == nil {
				stop(fmt.Errorf("ping: %w", err))
				return
			}
		}
	}
}
