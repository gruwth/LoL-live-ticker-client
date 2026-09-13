// Package updater checks for new releases and applies them, but only when the
// user asks and only when it is safe.
//
// Two rules shape everything here. Nothing downloads or executes without an
// explicit click, and nothing is applied while a game is live: the users of
// this tool are, by definition, in a match, and interrupting one would be
// unforgivable.
package updater

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/fynelabs/selfupdate"
)

// checkInterval is how often the background check runs. Updates are not
// urgent; this is deliberately unhurried.
const checkInterval = 6 * time.Hour

// State is what the UI renders: a quiet marker, never a modal.
type State struct {
	Available bool   // a newer release is signed, verified and ready
	Pending   bool   // the user asked for it; waiting for the game to end
	Message   string // set when something went wrong, shown verbatim
	Manual    string // download page, offered when applying failed
}

type Updater struct {
	source  selfupdate.Source
	pubKey  ed25519.PublicKey
	version string
	log     *slog.Logger

	mu       sync.Mutex
	state    State
	live     bool
	onChange func(State)
}

// New returns an updater, or nil if no public key is embedded in this build.
// A nil *Updater is safe to call every method on, so an unsigned developer
// build simply has no updater rather than a broken one.
func New(publicKey ed25519.PublicKey, releaseURL, version string, log *slog.Logger) *Updater {
	if len(publicKey) != ed25519.PublicKeySize || releaseURL == "" {
		log.Info("updates are disabled: this build has no release signing key")
		return nil
	}
	return &Updater{
		source:  selfupdate.NewHTTPSource(nil, releaseURL),
		pubKey:  publicKey,
		version: version,
		log:     log,
	}
}

// Enabled reports whether this build can update itself at all.
func (u *Updater) Enabled() bool { return u != nil }

// OnChange registers the UI callback. It fires from a background goroutine.
func (u *Updater) OnChange(fn func(State)) {
	if u == nil {
		return
	}
	u.mu.Lock()
	u.onChange = fn
	u.mu.Unlock()
}

// SetLive tells the updater whether a game is in progress. A queued update is
// applied as soon as one ends.
func (u *Updater) SetLive(live bool) {
	if u == nil {
		return
	}
	u.mu.Lock()
	was := u.live
	u.live = live
	pending := u.state.Pending
	u.mu.Unlock()

	if was && !live && pending {
		u.log.Info("game over; applying the update that was queued")
		go u.apply()
	}
}

// Start runs the scheduled check until ctx is done. It never applies anything:
// finding an update only raises a marker in the window and the tray.
func (u *Updater) Start(ctx context.Context) {
	if u == nil {
		return
	}
	_, err := selfupdate.Manage(&selfupdate.Config{
		Source:    u.source,
		Schedule:  selfupdate.Schedule{FetchOnStart: true, Interval: checkInterval},
		PublicKey: u.pubKey,

		// Always decline. This callback is the library asking "shall I apply
		// this now"; the answer during a scheduled check is always no, because
		// the user has not clicked anything. It is only used as the signal
		// that something is available.
		UpgradeConfirmCallback: func(msg string) bool {
			u.log.Info("an update is available", "detail", msg)
			u.set(func(s *State) { s.Available = true })
			return false
		},
	})
	if err != nil {
		u.log.Warn("update check could not start", "err", err)
		u.set(func(s *State) { s.Message = "Update checks are unavailable: " + err.Error() })
	}
	<-ctx.Done()
}

// Install is the explicit click. If a game is running the update is queued
// rather than applied, and the caller is told so.
func (u *Updater) Install() string {
	if u == nil {
		return "This build cannot update itself."
	}
	u.mu.Lock()
	live := u.live
	u.mu.Unlock()

	if live {
		u.set(func(s *State) { s.Pending = true })
		return "The update will be installed when your game ends."
	}
	go u.apply()
	return "Installing the update…"
}

// apply downloads, verifies the signature, and writes the new binary.
func (u *Updater) apply() {
	err := selfupdate.ManualUpdate(u.source, u.pubKey)
	if err == nil {
		u.log.Info("update applied; restart to finish")
		u.set(func(s *State) {
			s.Available, s.Pending = false, false
			s.Message = "Update installed. Restart the agent to finish."
		})
		return
	}

	// A half-updated binary that will not start is the worst outcome
	// available, so a failed rollback is surfaced loudly with a manual route
	// out rather than logged and forgotten.
	if rollback := selfupdate.RollbackError(err); rollback != nil {
		u.log.Error("update failed and could not roll back", "err", err, "rollback", rollback)
		u.set(func(s *State) {
			s.Pending = false
			s.Message = fmt.Sprintf("The update failed and the previous version could not be restored (%v). "+
				"Download a fresh copy to repair the install.", rollback)
			s.Manual = ReleasePage
		})
		return
	}

	u.log.Warn("update failed", "err", err)
	u.set(func(s *State) {
		s.Pending = false
		s.Message = "The update could not be installed: " + err.Error()
		s.Manual = ReleasePage
	})
}

func (u *Updater) set(mutate func(*State)) {
	u.mu.Lock()
	mutate(&u.state)
	snap := u.state
	fn := u.onChange
	u.mu.Unlock()
	if fn != nil {
		fn(snap)
	}
}

// State returns the current marker state.
func (u *Updater) State() State {
	if u == nil {
		return State{}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state
}

// AssetName is the release asset this platform downloads. Names are
// predictable so the source URL is a template rather than a manifest that has
// to be kept in step by hand.
func AssetName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("lolticker-agent-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
}
