package transform

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"lolticker-agent/internal/riot"
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
