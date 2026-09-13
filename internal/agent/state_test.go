package agent

import (
	"sync"
	"testing"
	"time"
)

// drain collects everything published within a short window.
func drain(ch <-chan State, d time.Duration) []State {
	var got []State
	deadline := time.After(d)
	for {
		select {
		case s := <-ch:
			got = append(got, s)
		case <-deadline:
			return got
		}
	}
}

func TestStatusIsDerivedFromBothHalves(t *testing.T) {
	tests := []struct {
		name   string
		relay  RelayState
		inGame bool
		want   Status
	}{
		{"nothing connected", RelayState{}, false, StatusOffline},
		{"connected, no game", RelayState{Connected: true}, false, StatusOnline},
		{"connected, in a game", RelayState{Connected: true}, true, StatusLive},
		{"an error outranks everything", RelayState{Connected: true, Err: "nope"}, true, StatusError},
		{"an error while offline is still an error", RelayState{Err: "nope"}, false, StatusError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newStates()
			s.setRelay(tc.relay)
			s.setGame(tc.inGame, "A#EUW", 12)
			if got := s.cur.Status; got != tc.want {
				t.Errorf("Status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPublishesOnlyOnChange(t *testing.T) {
	s := newStates()
	s.setRelay(RelayState{Connected: true})
	drain(s.ch, 20*time.Millisecond)

	// Same values again: nothing new to say.
	for i := 0; i < 5; i++ {
		s.setGame(false, "A#EUW", 0)
	}
	if got := drain(s.ch, 20*time.Millisecond); len(got) > 1 {
		t.Errorf("repeating an unchanged state published %d times, want at most 1", len(got))
	}

	// Sub-second clock movement is not a change either; GameTime is floored.
	s.setGame(true, "A#EUW", 30.1)
	drain(s.ch, 20*time.Millisecond)
	s.setGame(true, "A#EUW", 30.9)
	if got := drain(s.ch, 20*time.Millisecond); len(got) != 0 {
		t.Errorf("a sub-second clock change published %d times, want 0", len(got))
	}

	// A whole second is.
	s.setGame(true, "A#EUW", 31.0)
	if got := drain(s.ch, 20*time.Millisecond); len(got) != 1 {
		t.Errorf("a one-second clock change published %d times, want 1", len(got))
	}
}

// The channel must hand a slow reader the newest state, never a stale backlog.
func TestCoalescesForASlowConsumer(t *testing.T) {
	s := newStates()
	s.setRelay(RelayState{Connected: true})
	for i := 1; i <= 50; i++ {
		s.setGame(true, "A#EUW", float64(i))
	}

	got := drain(s.ch, 50*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("channel held %d values, want 1 (it must coalesce)", len(got))
	}
	if got[0].GameTime != 50 {
		t.Errorf("GameTime = %v, want the newest value 50", got[0].GameTime)
	}
}

func TestPublishingNeverBlocksWithNoConsumer(t *testing.T) {
	s := newStates()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			s.setGame(true, "A#EUW", float64(i))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("publishing blocked with nobody reading the channel")
	}
}

// Both halves report from their own goroutines while a front end reads. Run
// with -race; this is the test that makes the lock discipline worth anything.
func TestConcurrentReportersAndConsumer(t *testing.T) {
	s := newStates()
	var writers, consumer sync.WaitGroup
	stop := make(chan struct{})

	writers.Add(2)
	go func() {
		defer writers.Done()
		for i := 0; i < 2000; i++ {
			s.setRelay(RelayState{Connected: i%2 == 0, LatencyMS: i % 50, ShareURL: "https://x/y"})
		}
	}()
	go func() {
		defer writers.Done()
		for i := 0; i < 2000; i++ {
			s.setGame(i%3 == 0, "A#EUW", float64(i))
		}
	}()

	consumer.Add(1)
	go func() {
		defer consumer.Done()
		for {
			select {
			case <-stop:
				return
			case <-s.ch:
			}
		}
	}()

	writers.Wait()
	close(stop)
	consumer.Wait()
}
