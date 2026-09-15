package lcu

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

// A real LeagueClientUx command line, shortened. The flags appear in an order
// nobody controls, so the parse must not depend on it.
const sampleCmdline = `"C:\Riot Games\League of Legends\LeagueClientUx.exe" ` +
	`"--riotclient-auth-token=abc" "--riotclient-app-port=52000" ` +
	`"--remoting-auth-token=k7Yd-9_Qx0" "--app-port=61234" ` +
	`"--install-directory=C:\Riot Games\League of Legends" "--locale=en_GB"`

func TestParseCommandLine(t *testing.T) {
	c, ok := parseCommandLine(sampleCmdline)
	if !ok {
		t.Fatal("did not parse a valid command line")
	}
	if c.Port != 61234 {
		t.Errorf("port = %d, want 61234", c.Port)
	}
	if c.Token != "k7Yd-9_Qx0" {
		t.Errorf("token = %q, want k7Yd-9_Qx0", c.Token)
	}
}

// riotclient-app-port comes first in the string and must not be mistaken for
// app-port.
func TestParseDoesNotMatchTheRiotClientPort(t *testing.T) {
	if c, _ := parseCommandLine(sampleCmdline); c.Port == 52000 {
		t.Error("matched --riotclient-app-port instead of --app-port")
	}
}

func TestParseRejectsRubbish(t *testing.T) {
	for _, s := range []string{"", "notaleagueclient", "--app-port=61234", "--remoting-auth-token=x"} {
		if _, ok := parseCommandLine(s); ok {
			t.Errorf("accepted %q, want a rejection", s)
		}
	}
}

func TestParseLockfile(t *testing.T) {
	c, ok := parseLockfile("LeagueClient:12345:61234:k7Yd-9_Qx0:https\n")
	if !ok {
		t.Fatal("did not parse a valid lockfile")
	}
	if c.Port != 61234 || c.Token != "k7Yd-9_Qx0" {
		t.Errorf("got %+v", c)
	}
	for _, s := range []string{"", "a:b", "LeagueClient:1:notaport:tok:https", "LeagueClient:1:61234::https"} {
		if _, ok := parseLockfile(s); ok {
			t.Errorf("accepted malformed lockfile %q", s)
		}
	}
}

// The endpoint allowlist is the auditable claim of this whole feature, so it
// is enforced in code rather than left to reviewers to notice.
func TestOnlyTwoEndpointsAreAllowed(t *testing.T) {
	if len(allowedPaths) != 2 {
		t.Fatalf("the allowlist has %d entries, want exactly 2", len(allowedPaths))
	}
	for _, p := range []string{
		"/lol-chat/v1/friends",
		"/lol-match-history/v1/products/lol/current-summoner/matches",
		"/lol-summoner/v1/current-summoner",
		"/lol-gameflow/v1/session",
	} {
		if allowedPaths[p] {
			t.Errorf("%s is allowed and must not be", p)
		}
	}
}

// Discovery shells out to the OS process table, so a closed client must not
// cause that every poll.
func TestDiscoveryIsThrottledWhileTheClientIsClosed(t *testing.T) {
	c := New(slog.New(slog.NewTextHandler(io.Discard, nil)))

	start := time.Now()
	for i := 0; i < 20; i++ {
		if _, err := c.credentials(); err == nil {
			t.Skip("a League client is running, so this cannot be measured")
		}
	}
	// One scan is fine; twenty would take far longer than this.
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("20 lookups took %v, so the cooldown is not throttling them", elapsed)
	}
}
