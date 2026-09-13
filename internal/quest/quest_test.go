package quest

import (
	"testing"

	"lolticker-agent/internal/riot"
)

func spell(key string) riot.SummonerSpell {
	return riot.SummonerSpell{RawDisplayName: "GeneratedTip_SummonerSpell_" + key + "_DisplayName"}
}

func player(id string) riot.Player {
	return riot.Player{
		RiotID: id,
		Level:  11,
		Items:  make([]riot.Item, 4),
		SummonerSpells: riot.SummonerSpells{
			SummonerSpellOne: spell("SummonerFlash"),
			SummonerSpellTwo: spell(BaseTeleportKey),
		},
	}
}

func TestSpellKey(t *testing.T) {
	tests := map[string]string{
		"GeneratedTip_SummonerSpell_SummonerTeleport_DisplayName": "SummonerTeleport",
		"GeneratedTip_SummonerSpell_SummonerDot_DisplayName":      "SummonerDot",
		"": "",
	}
	for raw, want := range tests {
		if got := SpellKey(riot.SummonerSpell{RawDisplayName: raw}); got != want {
			t.Errorf("SpellKey(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestNothingFiresForAnOrdinaryPlayer(t *testing.T) {
	if got := NewTracker().Check([]riot.Player{player("A#EUW")}, 300); len(got) != 0 {
		t.Errorf("got %d events for a player with no quest done, want 0", len(got))
	}
}

func TestTopQuest(t *testing.T) {
	t.Run("upgraded teleport", func(t *testing.T) {
		p := player("A#EUW")
		p.SummonerSpells.SummonerSpellTwo = spell("S12_SummonerTeleportUpgrade")
		got := NewTracker().Check([]riot.Player{p}, 860)
		if len(got) != 1 || got[0].Quest != Top {
			t.Fatalf("got %+v, want one top quest", got)
		}
		e := got[0]
		if e.Name != EventName || !e.Synthetic || e.Killer != "A#EUW" || e.Time != 860 {
			t.Errorf("event = %+v", e)
		}
	})

	t.Run("level above the cap", func(t *testing.T) {
		p := player("A#EUW")
		p.Level = MaxLevelWithoutQuest + 1
		got := NewTracker().Check([]riot.Player{p}, 900)
		if len(got) != 1 || got[0].Quest != Top {
			t.Fatalf("got %+v, want one top quest", got)
		}
	})

	t.Run("level at the cap does not fire", func(t *testing.T) {
		p := player("A#EUW")
		p.Level = MaxLevelWithoutQuest
		if got := NewTracker().Check([]riot.Player{p}, 900); len(got) != 0 {
			t.Errorf("level %d fired %d events, want 0", p.Level, len(got))
		}
	})

	// The base spell must never look like an upgrade, or every top laner who
	// took Teleport would complete the quest at minute zero.
	t.Run("base teleport does not fire", func(t *testing.T) {
		if got := NewTracker().Check([]riot.Player{player("A#EUW")}, 60); len(got) != 0 {
			t.Errorf("base Teleport fired %d events, want 0", len(got))
		}
	})
}

func TestBotQuest(t *testing.T) {
	p := player("A#EUW")
	p.Items = make([]riot.Item, BotQuestItemCount) // six items plus a trinket
	if got := NewTracker().Check([]riot.Player{p}, 100); len(got) != 0 {
		t.Fatalf("a full normal inventory fired %d events, want 0", len(got))
	}

	p.Items = make([]riot.Item, BotQuestItemCount+1)
	got := NewTracker().Check([]riot.Player{p}, 1500)
	if len(got) != 1 || got[0].Quest != Bot {
		t.Fatalf("got %+v, want one bot quest", got)
	}
}

func TestMidQuest(t *testing.T) {
	// Tier3Boots ships empty until the IDs are verified against a real game,
	// so drive the detector through the same map a filled-in list would use.
	Tier3Boots[9999] = true
	defer delete(Tier3Boots, 9999)

	p := player("A#EUW")
	p.Items = []riot.Item{{ItemID: 1001}, {ItemID: 9999}}
	got := NewTracker().Check([]riot.Player{p}, 1200)
	if len(got) != 1 || got[0].Quest != Mid {
		t.Fatalf("got %+v, want one mid quest", got)
	}
}

// Selling and rebuying must not produce a second event.
func TestFiresOncePerPlayerPerGame(t *testing.T) {
	done := player("A#EUW")
	done.Items = make([]riot.Item, BotQuestItemCount+1)
	tr := NewTracker()

	if got := tr.Check([]riot.Player{done}, 1500); len(got) != 1 {
		t.Fatalf("first tick gave %d events, want 1", len(got))
	}
	if got := tr.Check([]riot.Player{done}, 1501); len(got) != 0 {
		t.Fatalf("second tick gave %d events, want 0", len(got))
	}

	sold := done
	sold.Items = make([]riot.Item, 5)
	tr.Check([]riot.Player{sold}, 1600)
	if got := tr.Check([]riot.Player{done}, 1700); len(got) != 0 {
		t.Errorf("rebuying re-emitted %d events, want 0", len(got))
	}

	tr.Reset()
	if got := tr.Check([]riot.Player{done}, 30); len(got) != 1 {
		t.Errorf("after Reset a new game gave %d events, want 1", len(got))
	}
}

// IDs must be unique, ordered, and far away from Riot's own EventID space.
func TestSyntheticIDs(t *testing.T) {
	a, b := player("A#EUW"), player("B#EUW")
	a.Items = make([]riot.Item, BotQuestItemCount+1)
	b.Items = make([]riot.Item, BotQuestItemCount+1)
	b.Level = MaxLevelWithoutQuest + 1

	tr := NewTracker()
	got := tr.Check([]riot.Player{a, b}, 1500)
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3 (A bot, B top, B bot)", len(got))
	}
	seen := map[int]bool{}
	for i, e := range got {
		if e.ID < FirstID {
			t.Errorf("event %d has ID %d, below the synthetic floor %d", i, e.ID, FirstID)
		}
		if seen[e.ID] {
			t.Errorf("duplicate synthetic ID %d", e.ID)
		}
		seen[e.ID] = true
	}

	tr.Reset()
	if tr.nextID != FirstID {
		t.Errorf("nextID = %d after Reset, want %d", tr.nextID, FirstID)
	}
}

func TestPlayersWithoutARiotIDAreSkipped(t *testing.T) {
	p := player("")
	p.Items = make([]riot.Item, BotQuestItemCount+1)
	if got := NewTracker().Check([]riot.Player{p}, 100); len(got) != 0 {
		t.Errorf("got %d events for a player with no Riot ID, want 0", len(got))
	}
}
