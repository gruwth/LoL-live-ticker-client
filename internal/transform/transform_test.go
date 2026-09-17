package transform

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"lolticker-agent/internal/riot"
	"lolticker-agent/internal/wire"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const goldenPath = "testdata/snapshot.golden.json"

func loadFixture(t *testing.T) *riot.AllGameData {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "allgamedata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var d riot.AllGameData
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	return &d
}

func TestSnapshotGolden(t *testing.T) {
	got, err := json.MarshalIndent(Snapshot(loadFixture(t), true), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("snapshot does not match %s\n--- got ---\n%s\n--- want ---\n%s", goldenPath, got, want)
	}
}

func TestSnapshotNoActive(t *testing.T) {
	if s := Snapshot(loadFixture(t), false); s.Active != nil {
		t.Errorf("Active = %+v, want nil when the user opted out", s.Active)
	}
}

// The lean form must be a fraction of the raw payload.
func TestSnapshotIsLean(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "allgamedata.json"))
	if err != nil {
		t.Fatal(err)
	}
	lean, err := json.Marshal(Snapshot(loadFixture(t), true))
	if err != nil {
		t.Fatal(err)
	}
	if len(lean) >= len(raw)/2 {
		t.Errorf("lean snapshot is %d bytes against %d raw, expected a much bigger cut", len(lean), len(raw))
	}
}

func ev(id int, name string) riot.Event {
	return riot.Event{EventID: id, EventName: name, EventTime: float64(id) * 10}
}

func TestEvents(t *testing.T) {
	a, b, c := ev(0, "GameStart"), ev(1, "MinionsSpawning"), ev(2, "FirstBrick")
	zeroA, zeroB, zeroC := ev(0, "GameStart"), ev(0, "MinionsSpawning"), ev(0, "FirstBrick")

	tests := []struct {
		name     string
		raw      []riot.Event
		lastSeen int
		want     []string
		wantLast int
	}{
		{"first tick of a game", []riot.Event{a}, NoEventsSeen, []string{"GameStart"}, 0},
		{"same batch again emits nothing", []riot.Event{a}, 0, nil, 0},
		{"only the new one", []riot.Event{a, b, c}, 1, []string{"FirstBrick"}, 2},
		{"nothing new", []riot.Event{a, b, c}, 2, nil, 2},
		{"no events at all", nil, 5, nil, 5},
		{"all-zero ids: first batch", []riot.Event{zeroA, zeroB}, NoEventsSeen, []string{"GameStart", "MinionsSpawning"}, 2},
		{"all-zero ids: only the new one", []riot.Event{zeroA, zeroB, zeroC}, 2, []string{"FirstBrick"}, 3},
		{"all-zero ids: nothing new", []riot.Event{zeroA, zeroB}, 2, nil, 2},
		{"game restart re-emits everything", []riot.Event{a, b}, 17, []string{"GameStart", "MinionsSpawning"}, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, last := Events(tc.raw, tc.lastSeen)
			names := make([]string, 0, len(got))
			for _, e := range got {
				names = append(names, e.Name)
			}
			if len(names) != len(tc.want) {
				t.Fatalf("events = %v, want %v", names, tc.want)
			}
			for i := range names {
				if names[i] != tc.want[i] {
					t.Fatalf("events = %v, want %v", names, tc.want)
				}
			}
			if last != tc.wantLast {
				t.Errorf("lastSeen = %d, want %d", last, tc.wantLast)
			}
		})
	}
}

func TestEventsFields(t *testing.T) {
	raw := loadFixture(t).Events.Events
	got, last := Events(raw, NoEventsSeen)
	if len(got) != len(raw) {
		t.Fatalf("got %d events, want %d", len(got), len(raw))
	}
	if last != 4 {
		t.Errorf("lastSeen = %d, want 4", last)
	}

	dragon := got[3]
	if dragon.Name != "DragonKill" || dragon.Dragon != "Infernal" || dragon.Killer != "Gruwth#EUW" {
		t.Errorf("dragon event = %+v", dragon)
	}
	if dragon.Stolen {
		t.Errorf(`Stolen = true, want false for "False"`)
	}

	// A second pass over the same batch must produce nothing.
	if again, _ := Events(raw, last); len(again) != 0 {
		t.Errorf("re-polling the same batch emitted %d events", len(again))
	}
}

func TestEventsStolenParsed(t *testing.T) {
	raw := []riot.Event{{EventID: 1, EventName: "BaronKill", Stolen: "True"}}
	got, _ := Events(raw, NoEventsSeen)
	if len(got) != 1 || !got[0].Stolen {
		t.Errorf(`Stolen not parsed from "True": %+v`, got)
	}
}

// FirstBlood is the one event that names its player in Recipient rather than
// KillerName. Riot's sample event list omits FirstBlood entirely, so nothing in
// the spec said so and the event reached the frontend with no actor at all.
func TestEventsFirstBloodTakesRecipient(t *testing.T) {
	raw := []riot.Event{{EventID: 1, EventName: "FirstBlood", Recipient: "table for one"}}
	got, _ := Events(raw, NoEventsSeen)
	if len(got) != 1 || got[0].Killer != "table for one" {
		t.Errorf("FirstBlood killer = %+v, want Recipient", got)
	}
}

// Each inhibitor event names the structure under its own event name, so a
// respawn carries nothing under InhibKilled. All three have to land on the one
// wire field, or the frontend cannot say which inhibitor came back.
func TestEventsInhibNameFromEveryInhibEvent(t *testing.T) {
	const name = "Barracks_T2_L1"
	raw := []riot.Event{
		{EventID: 1, EventName: "InhibKilled", InhibKilled: name},
		{EventID: 2, EventName: "InhibRespawningSoon", InhibRespawningSoon: name},
		{EventID: 3, EventName: "InhibRespawned", InhibRespawned: name},
	}
	got, _ := Events(raw, NoEventsSeen)
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	for _, e := range got {
		if e.Inhib != name {
			t.Errorf("%s carried inhib %q, want %q", e.Name, e.Inhib, name)
		}
	}
}

func TestFullRunesSplit(t *testing.T) {
	s := Snapshot(loadFixture(t), true)
	if s.Active == nil || s.Active.FullRunes == nil {
		t.Fatal("fullRunes missing from active")
	}
	fr := s.Active.FullRunes

	if fr.Keystone.ID != 8229 || fr.Keystone.Name != "Arcane Comet" {
		t.Errorf("keystone = %+v", fr.Keystone)
	}
	if fr.PrimaryTree != "Sorcery" || fr.SecondaryTree != "Domination" {
		t.Errorf("trees = %q / %q", fr.PrimaryTree, fr.SecondaryTree)
	}
	// generalRunes is [keystone, 3 primary, 2 secondary] with nothing marking
	// the boundaries, so this split is the part that can silently go wrong.
	if len(fr.Primary) != 3 || len(fr.Secondary) != 2 {
		t.Fatalf("split gave %d primary and %d secondary, want 3 and 2", len(fr.Primary), len(fr.Secondary))
	}
	if fr.Primary[0].Name != "Manaflow Band" || fr.Primary[2].Name != "Scorch" {
		t.Errorf("primary = %+v", fr.Primary)
	}
	if fr.Secondary[0].Name != "Relentless Hunter" || fr.Secondary[1].Name != "Taste of Blood" {
		t.Errorf("secondary = %+v", fr.Secondary)
	}
	// The keystone must not also show up as a minor rune.
	for _, r := range append(append([]wire.Rune{}, fr.Primary...), fr.Secondary...) {
		if r.ID == fr.Keystone.ID {
			t.Errorf("keystone %d leaked into the minor runes", r.ID)
		}
	}
	if want := []int{5008, 5008, 5001}; len(fr.Shards) != 3 {
		t.Errorf("shards = %v, want %v", fr.Shards, want)
	}
}

// A short or empty rune block must not panic the positional split.
func TestFullRunesTolerateShortLists(t *testing.T) {
	for n := 0; n <= 6; n++ {
		raw := loadFixture(t)
		raw.ActivePlayer.FullRunes.GeneralRunes = raw.ActivePlayer.FullRunes.GeneralRunes[:n]
		got := Snapshot(raw, true)
		if got.Active == nil {
			t.Fatalf("n=%d: active went missing", n)
		}
		fr := got.Active.FullRunes
		if fr == nil {
			continue // only valid when there is genuinely nothing
		}
		if len(fr.Primary) > 3 || len(fr.Secondary) > 2 {
			t.Errorf("n=%d: split overran: %d primary, %d secondary", n, len(fr.Primary), len(fr.Secondary))
		}
	}
}

// The privacy toggle has to gate the rune page with no extra code path.
func TestFullRunesGatedByShareActive(t *testing.T) {
	if s := Snapshot(loadFixture(t), false); s.Active != nil {
		t.Fatal("active survived the opt-out, so fullRunes would leak with it")
	}
}
