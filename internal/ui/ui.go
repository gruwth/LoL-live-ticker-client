//go:build !nogui

// Package ui is the desktop front end. It depends on the core; the core knows
// nothing about it. Everything here is behind the !nogui build tag, so a
// headless build never compiles a line of Fyne and never needs cgo.
package ui

import (
	"context"
	"fmt"
	"image/color"
	"net/url"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"lolticker-agent/internal/agent"
	"lolticker-agent/internal/autostart"
	"lolticker-agent/internal/config"
)

// AppID must be a fixed reverse-DNS string: NewWithID, not New, is what makes
// Fyne's preference storage work at all.
const AppID = "dev.erxt.lolticker"

// TokenPage is where a user goes to generate the token.
const TokenPage = "https://lol.erxt.dev/settings"

// Controller is everything the window needs from the rest of the app. Keeping
// it an interface means the UI never reaches into the agent or the relay.
type Controller interface {
	// States is the coalescing channel the core publishes to.
	States() <-chan agent.State
	// Config returns the current settings.
	Config() config.Config
	// Save persists settings and applies them live, without a restart.
	Save(config.Config) error
	// ConfigPath is shown to the user so they can find the file if they want.
	ConfigPath() string
	// LogText is the tail of the log, already redacted.
	LogText() string
	// Version is the build version string.
	Version() string
	// CheckForUpdates is wired in the v3 updater step.
	CheckForUpdates()
	// Quit shuts the whole app down.
	Quit()
}

type UI struct {
	ctrl      Controller
	autostart autostart.Interface

	app  fyne.App
	win  fyne.Window
	desk desktop.App

	// Status block
	dot        *canvas.Circle
	statusText *widget.Label
	detail     *widget.Label
	errText    *widget.Label

	// Share link
	shareLabel *widget.Label
	copyBtn    *widget.Button
	openBtn    *widget.Button

	// Token
	tokenEntry *widget.Entry

	logView *widget.Entry

	cur agent.State
}

func New(ctrl Controller) *UI {
	return &UI{ctrl: ctrl, autostart: autostart.New()}
}

// Run builds the window and blocks on the Fyne event loop until Quit.
// It must be called from the main goroutine.
func (u *UI) Run(ctx context.Context, startMinimized bool) {
	u.app = app.NewWithID(AppID)
	u.win = u.app.NewWindow("LoL Live Ticker")
	u.win.SetContent(u.build())
	u.win.Resize(fyne.NewSize(480, 620))

	// Closing hides to the tray; only the tray's Quit really exits. Users
	// assume the X button quit the app, so say so the first time.
	u.win.SetCloseIntercept(func() {
		u.win.Hide()
		u.noteHiddenOnce()
	})

	u.buildTray()
	go u.consume(ctx)

	if startMinimized {
		// Run rather than ShowAndRun, and never Show: that is what "start in
		// the tray" means.
		u.app.Run()
		return
	}
	u.win.ShowAndRun()
}

// Show raises the window. Safe to call from any goroutine, which matters
// because the single-instance listener calls it.
func (u *UI) Show() {
	fyne.Do(func() {
		u.win.Show()
		u.win.RequestFocus()
	})
}

// refreshLog pulls the log tail into the view. The log moves independently of
// the state, so this is also on a timer: an app that is quietly connected
// publishes no state changes but is still writing lines.
func (u *UI) refreshLog() {
	text := u.ctrl.LogText()
	if text == u.logView.Text {
		return
	}
	u.logView.SetText(text)
}

// consume pumps core state into the widgets. Every touch of a widget goes
// through fyne.Do, because Fyne requires UI work on its own goroutine.
func (u *UI) consume(ctx context.Context) {
	states := u.ctrl.States()
	logTick := time.NewTicker(2 * time.Second)
	defer logTick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case st := <-states:
			fyne.Do(func() { u.apply(st) })
		case <-logTick.C:
			fyne.Do(u.refreshLog)
		}
	}
}

func (u *UI) apply(st agent.State) {
	prev := u.cur
	u.cur = st

	u.dot.FillColor = colorForStatus(st.Status)
	u.dot.Refresh()
	u.statusText.SetText(labelForStatus(st.Status))

	switch st.Status {
	case agent.StatusLive:
		d := time.Duration(st.GameTime) * time.Second
		line := fmt.Sprintf("%s  ·  %02d:%02d", st.RiotID, int(d.Minutes()), int(d.Seconds())%60)
		if st.LatencyMS > 0 {
			line += fmt.Sprintf("  ·  %d ms", st.LatencyMS)
		}
		u.detail.SetText(line)
	case agent.StatusOnline:
		if st.LatencyMS > 0 {
			u.detail.SetText(fmt.Sprintf("Connected  ·  %d ms", st.LatencyMS))
		} else {
			u.detail.SetText("Connected, waiting for a game")
		}
	default:
		u.detail.SetText("")
	}
	// An empty label still occupies a row, which leaves a dead gap under the
	// status line.
	if u.detail.Text == "" {
		u.detail.Hide()
	} else {
		u.detail.Show()
	}

	// An auth failure or a relay rejection has to be readable here, not buried
	// in a log file.
	u.errText.SetText(st.LastError)
	if st.LastError == "" {
		u.errText.Hide()
	} else {
		u.errText.Show()
	}

	u.setShare(st.ShareURL)
	u.refreshLog()

	if prev.Status != st.Status {
		u.refreshTray()
	}
}

func (u *UI) setShare(share string) {
	if share == "" {
		u.shareLabel.SetText("Available once you have played a game")
		u.copyBtn.Disable()
		u.openBtn.Disable()
		return
	}
	u.shareLabel.SetText(share)
	u.copyBtn.Enable()
	u.openBtn.Enable()
}

func colorForStatus(s agent.Status) color.Color {
	switch s {
	case agent.StatusOnline:
		return color.RGBA{0x3f, 0xb9, 0x50, 0xff}
	case agent.StatusLive:
		return color.RGBA{0x35, 0x9d, 0xf5, 0xff}
	case agent.StatusError:
		return color.RGBA{0xd9, 0x3a, 0x3a, 0xff}
	default:
		return color.RGBA{0x8a, 0x8f, 0x98, 0xff}
	}
}

func labelForStatus(s agent.Status) string {
	switch s {
	case agent.StatusOnline:
		return "Online — not in a game"
	case agent.StatusLive:
		return "In a game — streaming"
	case agent.StatusError:
		return "Something is wrong"
	default:
		return "Offline"
	}
}

func (u *UI) openURL(raw string) {
	if raw == "" {
		return
	}
	if parsed, err := url.Parse(raw); err == nil {
		if err := u.app.OpenURL(parsed); err != nil {
			dialog.ShowError(err, u.win)
		}
	}
}

// openConfigDir shows the config file in the system file manager, which is
// friendlier than printing a path the user then has to copy by hand.
func (u *UI) openConfigDir() {
	dir := filepath.Dir(u.ctrl.ConfigPath())
	if parsed, err := url.Parse("file://" + filepath.ToSlash(dir)); err == nil {
		if err := u.app.OpenURL(parsed); err != nil {
			dialog.ShowInformation("Config file", u.ctrl.ConfigPath(), u.win)
		}
	}
}

func (u *UI) noteHiddenOnce() {
	const key = "toldAboutTray"
	if u.app.Preferences().Bool(key) {
		return
	}
	u.app.Preferences().SetBool(key, true)
	u.app.SendNotification(fyne.NewNotification(
		"Still running",
		"The agent is in your tray and still streaming. Use Quit in the tray menu to stop it.",
	))
}

// errorDialog is used by the settings handlers, which all fail the same way.
func (u *UI) fail(err error) {
	if err != nil {
		dialog.ShowError(err, u.win)
	}
}
