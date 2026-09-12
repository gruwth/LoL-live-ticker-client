// Package riot is the only thing in this program that talks to the game.
// It reads https://127.0.0.1:2999/liveclientdata and nothing else.
package riot

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"
)

const BaseURL = "https://127.0.0.1:2999/liveclientdata"

// maxBody caps a raw allgamedata read. Live payloads are 40-80 KB.
const maxBody = 4 << 20

var (
	// ErrNoGame means there is no live match to read: either nothing is
	// listening on 2999, or the game process is up but the match has not
	// started. Both are normal.
	ErrNoGame = errors.New("no live game")
	// ErrDecode means the response did not parse - most likely the schema
	// changed. Callers should log loudly and keep their last good state.
	ErrDecode = errors.New("decode allgamedata")
)

type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 2 * time.Second,
			Transport: &http.Transport{
				// The game client serves a self-signed cert for *.riotgames.com
				// on a loopback address, so hostname verification can never pass.
				// Scoped to this client only; it is never used for any other host.
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// AllGameData fetches and decodes /allgamedata.
func (c *Client) AllGameData(ctx context.Context) (*AllGameData, error) {
	body, err := c.get(ctx, "/allgamedata")
	if err != nil {
		return nil, err
	}
	var d AllGameData
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecode, err)
	}
	return &d, nil
}

// Raw returns the undecoded response body for a liveclientdata path.
// Used by --dump so a bug report can ship exactly what the client sent.
func (c *Client) Raw(ctx context.Context, path string) ([]byte, error) {
	return c.get(ctx, path)
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if isDialError(err) {
			return nil, fmt.Errorf("%w: client not running: %v", ErrNoGame, err)
		}
		// Timeouts land here: the client is busy (loading screen, alt-tab).
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound && bytes.Contains(body, []byte("RESOURCE_NOT_FOUND")) {
		return nil, fmt.Errorf("%w: match not started", ErrNoGame)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("riot api %s: http %d", path, resp.StatusCode)
	}
	return body, nil
}

// IsGameNotRunning reports whether err just means "there is no match to read
// right now", which is the normal idle condition and not worth logging.
func IsGameNotRunning(err error) bool { return errors.Is(err, ErrNoGame) }

func isDialError(err error) bool {
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		return true
	}
	return false
}
