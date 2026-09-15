package lcu

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"
)

// The complete list of endpoints this agent may read. Any other path is a bug,
// and get refuses to fetch one — the README makes this promise to users, so it
// is enforced here rather than left to reviewers to notice.
const (
	PathGameflowPhase = "/lol-gameflow/v1/gameflow-phase"
	PathChampSelect   = "/lol-champ-select/v1/session"
)

var allowedPaths = map[string]bool{
	PathGameflowPhase: true,
	PathChampSelect:   true,
}

const requestTimeout = 2 * time.Second

// discoverCooldown throttles the search for a client that is not there.
// Discovery shells out to the OS process table, which is far too expensive to
// repeat at the poll rate for the many hours a day the client is closed.
const discoverCooldown = 30 * time.Second

// ErrNoChampSelect means the client answered but there is no session, which is
// the normal state outside champ select.
var ErrNoChampSelect = errors.New("not in champ select")

type Client struct {
	log *slog.Logger

	mu       sync.Mutex
	creds    Credentials
	notUntil time.Time // suppress discovery while the client is known to be closed
	http     *http.Client
}

func New(log *slog.Logger) *Client {
	return &Client{
		log: log,
		http: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				// The client serves a self-signed cert on loopback, exactly
				// like the game API. Scoped to this client only; never used
				// for any other host.
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// GameflowPhase returns the raw phase string, e.g. "ChampSelect".
func (c *Client) GameflowPhase(ctx context.Context) (string, error) {
	body, err := c.get(ctx, PathGameflowPhase)
	if err != nil {
		return "", err
	}
	var phase string
	if err := json.Unmarshal(body, &phase); err != nil {
		return "", fmt.Errorf("decode gameflow phase: %w", err)
	}
	return phase, nil
}

// ChampSelect returns the current champ select session.
func (c *Client) ChampSelect(ctx context.Context) (*Session, error) {
	body, err := c.get(ctx, PathChampSelect)
	if errors.Is(err, errNotFound) {
		return nil, ErrNoChampSelect
	}
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("decode champ select session: %w", err)
	}
	return &s, nil
}

var errNotFound = errors.New("not found")

// get makes one request, re-discovering credentials once if the cached pair
// has gone stale.
//
// The port and token change on every client restart. Caching them for the
// session would mean the agent silently stops working the first time someone
// restarts their client, and that looks like a server bug rather than a local
// one — so a connection failure or a 401 discards them and retries.
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	if !allowedPaths[path] {
		return nil, fmt.Errorf("refusing to read %q: not one of the two allowed endpoints", path)
	}

	body, err := c.try(ctx, path)
	if err == nil || !staleCredentials(err) {
		return body, err
	}

	c.log.Debug("league client credentials look stale; re-discovering", "err", err)
	c.forget()
	return c.try(ctx, path)
}

func (c *Client) try(ctx context.Context, path string) ([]byte, error) {
	creds, err := c.credentials()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://127.0.0.1:%d%s", creds.Port, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("riot", creds.Token)

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	case http.StatusNotFound:
		return nil, errNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, errUnauthorized
	default:
		return nil, fmt.Errorf("lcu %s: http %d", path, resp.StatusCode)
	}
}

var errUnauthorized = errors.New("lcu rejected the token")

// staleCredentials reports whether err is the kind that a fresh port and token
// would fix.
func staleCredentials(err error) bool {
	if errors.Is(err, errUnauthorized) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}

func (c *Client) credentials() (Credentials, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.creds.Port != 0 {
		return c.creds, nil
	}
	if time.Now().Before(c.notUntil) {
		return Credentials{}, ErrNotRunning
	}

	creds, err := Discover()
	if err != nil {
		c.notUntil = time.Now().Add(discoverCooldown)
		return Credentials{}, err
	}
	c.creds = creds
	c.notUntil = time.Time{}
	c.log.Debug("found the league client", "port", creds.Port)
	return creds, nil
}

// forget drops the cached pair so the next call re-discovers. The cooldown is
// cleared too: a stale token means the client restarted, which is exactly when
// looking again is worthwhile.
func (c *Client) forget() {
	c.mu.Lock()
	c.creds = Credentials{}
	c.notUntil = time.Time{}
	c.mu.Unlock()
}

// IsNotRunning reports whether err just means the client is closed, which is
// the normal state for most of the day.
func IsNotRunning(err error) bool {
	return errors.Is(err, ErrNotRunning) || errors.Is(err, syscall.ECONNREFUSED)
}
