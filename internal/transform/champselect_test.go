package transform

import (
	"encoding/json"
	"strings"
	"testing"

	"lolticker-agent/internal/lcu"
)

func TestPhaseMapping(t *testing.T) {
	cases := map[string]string{
		"None": "none", "": "none",
		"Lobby": "lobby", "Matchmaking": "matchmaking", "ReadyCheck": "readycheck",
		"ChampSelect": "champselect",
		"GameStart":   "ingame", "InProgress": "ingame", "Reconnect": "ingame",
		"WaitingForStats": "endofgame", "PreEndOfGame": "endofgame", "EndOfGame": "endofgame",
		// Riot adds phases; an unrecognised one is passed through lowercased
		// rather than dropped.
		"SomeNewPhase": "somenewphase",
	}
	for in, want := range cases {
		if got := Phase(in).Phase; got != want {
			t.Errorf("Phase(%q) = %q, want %q", in, got, want)
		}
	}
}

// session builds a draft where cell 1 has locked Ahri and cell 2 is only
// hovering Zed.
func session() *lcu.Session {
	return &lcu.Session{
		LocalPlayerCellID: 1,
		MyTeam: []lcu.Player{
			{CellID: 0},
			{CellID: 1, ChampionID: 103, ChampionPickIntent: 103},
			// A hover lives in these two fields, and they must never be read.
			{CellID: 2, ChampionID: 238, ChampionPickIntent: 238},
		},
		TheirTeam: []lcu.Player{{CellID: 5}, {CellID: 6, ChampionID: 64, ChampionPickIntent: 64}},
		Actions: [][]lcu.Action{
			{{ActorCellID: 0, ChampionID: 84, Completed: true, Type: "ban"}},
			{{ActorCellID: 1, ChampionID: 103, Completed: true, Type: "pick"}},
			// Not completed: this is the hover.
			{{ActorCellID: 2, ChampionID: 238, Completed: false, Type: "pick"}},
			{{ActorCellID: 6, ChampionID: 64, Completed: false, Type: "pick"}},
		},
	}
}

// The rule that makes this publishable: a hovered champion must not appear.
func TestHoversNeverReachTheWire(t *testing.T) {
	cs := ChampSelect(session(), "Owner#EUW")

	blob, _ := json.Marshal(cs)
	for _, hovered := range []string{"238", "64"} {
		if strings.Contains(string(blob), hovered) {
			t.Errorf("hovered champion %s leaked into the frame: %s", hovered, blob)
		}
	}

	if got := cs.MyTeam[2]; got.ChampionID != 0 || got.Locked {
		t.Errorf("hovering seat = %+v, want championId 0 and locked false", got)
	}
	if got := cs.TheirTeam[1]; got.ChampionID != 0 || got.Locked {
		t.Errorf("enemy hovering seat = %+v, want championId 0 and locked false", got)
	}

	// The locked pick does come through.
	if got := cs.MyTeam[1]; got.ChampionID != 103 || !got.Locked {
		t.Errorf("locked seat = %+v, want championId 103 and locked true", got)
	}
}

// The other rule: no non-party real name, in any queue.
func TestOnlyTheOwnerIsNamed(t *testing.T) {
	cs := ChampSelect(session(), "Owner#EUW")

	if cs.MyTeam[1].Label != "Owner#EUW" || !cs.MyTeam[1].IsOwner {
		t.Errorf("owner seat = %+v, want the owner's own name", cs.MyTeam[1])
	}
	for i, p := range cs.MyTeam {
		if i == 1 {
			continue
		}
		if p.IsOwner {
			t.Errorf("seat %d claims to be the owner", i)
		}
		if !strings.HasPrefix(p.Label, "Ally ") {
			t.Errorf("ally seat %d labelled %q, want a positional label", i, p.Label)
		}
	}
	for i, p := range cs.TheirTeam {
		if !strings.HasPrefix(p.Label, "Enemy ") {
			t.Errorf("enemy seat %d labelled %q, want a positional label", i, p.Label)
		}
	}
}

// Without an owner name the owner seat still must not fall back to anything
// identifying.
func TestNoOwnerNameStillLabelsPositionally(t *testing.T) {
	cs := ChampSelect(session(), "")
	if got := cs.MyTeam[1]; got.Label != "Ally 2" || !got.IsOwner {
		t.Errorf("owner seat with no name = %+v, want a positional label and isOwner", got)
	}
}

func TestBansTravelFreely(t *testing.T) {
	cs := ChampSelect(session(), "Owner#EUW")
	if len(cs.Bans) != 1 || cs.Bans[0] != 84 {
		t.Errorf("bans = %v, want [84]", cs.Bans)
	}
}

// An empty session must not panic or produce nulls the relay would reject.
func TestEmptySession(t *testing.T) {
	cs := ChampSelect(&lcu.Session{}, "")
	if cs.Bans == nil || cs.MyTeam == nil || cs.TheirTeam == nil {
		t.Errorf("empty session produced nil slices: %+v", cs)
	}
}
