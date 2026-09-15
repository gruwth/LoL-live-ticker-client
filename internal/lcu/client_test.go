package lcu

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
)

// fakeLCU stands in for the League client: same self-signed TLS on loopback,
// same basic auth, same shapes.
func fakeLCU(t *testing.T, h http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}

	c := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.creds = Credentials{Port: port, Token: "test-token"}
	return c, srv
}

func TestGameflowPhase(t *testing.T) {
	c, _ := fakeLCU(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "riot" || pass != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != PathGameflowPhase {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		io.WriteString(w, `"ChampSelect"`)
	}))

	got, err := c.GameflowPhase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "ChampSelect" {
		t.Errorf("phase = %q, want ChampSelect", got)
	}
}

// Outside champ select the endpoint 404s, which is normal and not an error
// worth surfacing.
func TestChampSelectWhenThereIsNoSession(t *testing.T) {
	c, _ := fakeLCU(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	if _, err := c.ChampSelect(context.Background()); !errors.Is(err, ErrNoChampSelect) {
		t.Errorf("err = %v, want ErrNoChampSelect", err)
	}
}

func TestChampSelectDecodes(t *testing.T) {
	const body = `{
	  "localPlayerCellId": 1,
	  "myTeam": [{"cellId":0,"championId":0},{"cellId":1,"championId":103,"championPickIntent":103}],
	  "theirTeam": [{"cellId":5,"championId":0,"championPickIntent":238}],
	  "actions": [
	    [{"actorCellId":0,"championId":84,"completed":true,"type":"ban"}],
	    [{"actorCellId":1,"championId":103,"completed":true,"type":"pick"}],
	    [{"actorCellId":5,"championId":238,"completed":false,"type":"pick"}]
	  ]
	}`
	c, _ := fakeLCU(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))

	s, err := c.ChampSelect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.LocalPlayerCellID != 1 {
		t.Errorf("localPlayerCellId = %d, want 1", s.LocalPlayerCellID)
	}
	picks := s.CompletedPicks()
	if picks[1] != 103 {
		t.Errorf("completed pick for cell 1 = %d, want 103", picks[1])
	}
	if _, hovering := picks[5]; hovering {
		t.Error("an incomplete pick was treated as locked")
	}
	if bans := s.Bans(); len(bans) != 1 || bans[0] != 84 {
		t.Errorf("bans = %v, want [84]", bans)
	}
}

// The port and token change on every client restart, so a 401 has to trigger
// re-discovery rather than failing for the rest of the session.
func TestStaleTokenTriggersRediscovery(t *testing.T) {
	var calls atomic.Int32
	c, _ := fakeLCU(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized) // the cached token has gone stale
			return
		}
		io.WriteString(w, `"Lobby"`)
	}))

	// Discovery will fail here (no real client), so the retry surfaces that
	// rather than the 401. What matters is that it re-discovered at all.
	_, err := c.GameflowPhase(context.Background())
	if calls.Load() < 1 {
		t.Fatal("no request was made")
	}
	c.mu.Lock()
	cached := c.creds.Port
	c.mu.Unlock()
	if cached != 0 {
		t.Errorf("stale credentials were kept after a 401 (port %d)", cached)
	}
	if err == nil {
		t.Log("retry succeeded; a League client must be running")
	}
}

// The allowlist is enforced at the point of use, not just documented.
func TestGetRefusesAnEndpointOutsideTheAllowlist(t *testing.T) {
	c, _ := fakeLCU(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a request reached the server for %q", r.URL.Path)
	}))
	if _, err := c.get(context.Background(), "/lol-chat/v1/friends"); err == nil {
		t.Fatal("reading a chat endpoint was allowed")
	}
}
