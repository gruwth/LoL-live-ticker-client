//go:build !nogui

package ui

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"lolticker-agent/internal/autostart"
)

// build lays out the single window. The status block comes first because "is
// this thing working" is the only question most users will ever have.
func (u *UI) build() fyne.CanvasObject {
	return container.NewVBox(
		u.buildStatus(),
		widget.NewSeparator(),
		u.buildShare(),
		widget.NewSeparator(),
		u.buildToken(),
		widget.NewSeparator(),
		u.buildToggles(),
		widget.NewSeparator(),
		u.buildLog(),
		u.buildFooter(),
	)
}

func (u *UI) buildStatus() fyne.CanvasObject {
	// GridWrap pins the circle to an exact size; a bare canvas object in a
	// VBox would stretch to whatever space it was given.
	u.dot = canvas.NewCircle(colorForStatus(""))
	dotBox := container.NewGridWrap(fyne.NewSize(14, 14), u.dot)

	u.statusText = widget.NewLabel(labelForStatus(""))
	u.statusText.TextStyle = fyne.TextStyle{Bold: true}

	u.detail = widget.NewLabel("")
	u.errText = widget.NewLabel("")
	u.errText.Wrapping = fyne.TextWrapWord
	u.errText.Hide()
	u.detail.Hide()

	return container.NewVBox(
		container.NewHBox(dotBox, u.statusText),
		u.detail,
		u.errText,
	)
}

func (u *UI) buildShare() fyne.CanvasObject {
	u.shareLabel = widget.NewLabel("Available once you have played a game")
	u.shareLabel.Wrapping = fyne.TextWrapBreak

	u.copyBtn = widget.NewButton("Copy", func() {
		u.app.Clipboard().SetContent(u.cur.ShareURL)
	})
	u.openBtn = widget.NewButton("Open", func() { u.openURL(u.cur.ShareURL) })
	u.copyBtn.Disable()
	u.openBtn.Disable()

	return container.NewVBox(
		boldLabel("Your page"),
		u.shareLabel,
		container.NewHBox(u.copyBtn, u.openBtn),
	)
}

// buildToken is the first-run path: the entire point of the GUI is that a new
// user never has to find and hand-edit a JSON file.
func (u *UI) buildToken() fyne.CanvasObject {
	u.tokenEntry = widget.NewPasswordEntry()
	u.tokenEntry.SetPlaceHolder("lt_...")
	u.tokenEntry.SetText(u.ctrl.Config().Token)

	save := widget.NewButton("Save and connect", func() {
		cfg := u.ctrl.Config()
		cfg.Token = u.tokenEntry.Text
		if err := u.ctrl.Save(cfg); err != nil {
			u.fail(err)
			return
		}
		dialog.ShowInformation("Saved", "Reconnecting with the new token.", u.win)
	})
	save.Importance = widget.HighImportance

	link := widget.NewButton("Get a token", func() { u.openURL(TokenPage) })
	link.Importance = widget.LowImportance

	return container.NewVBox(
		boldLabel("Token"),
		u.tokenEntry,
		container.NewHBox(save, link),
	)
}

func (u *UI) buildToggles() fyne.CanvasObject {
	cfg := u.ctrl.Config()

	share := widget.NewCheck("Share my gold, stats and abilities", func(v bool) {
		c := u.ctrl.Config()
		c.ShareActivePlayer = v
		u.fail(u.ctrl.Save(c))
	})
	share.SetChecked(cfg.ShareActivePlayer)

	discord := widget.NewCheck("Discord Rich Presence", func(v bool) {
		c := u.ctrl.Config()
		c.DiscordRPC = v
		u.fail(u.ctrl.Save(c))
	})
	discord.SetChecked(cfg.DiscordRPC)

	minimised := widget.NewCheck("Start minimised to tray", func(v bool) {
		c := u.ctrl.Config()
		c.StartMinimized = v
		u.fail(u.ctrl.Save(c))
	})
	minimised.SetChecked(cfg.StartMinimized)

	updates := widget.NewCheck("Check for updates automatically", func(v bool) {
		c := u.ctrl.Config()
		c.AutoCheckUpdates = v
		u.fail(u.ctrl.Save(c))
	})
	updates.SetChecked(cfg.AutoCheckUpdates)

	items := []fyne.CanvasObject{boldLabel("Settings"), share, discord, minimised, updates}

	// macOS has no implementation, so the checkbox is hidden rather than shown
	// doing nothing.
	if u.autostart.Supported() {
		login := widget.NewCheck("Start automatically on login", func(v bool) {
			var err error
			if v {
				err = u.autostart.Enable()
			} else {
				err = u.autostart.Disable()
			}
			if err != nil && !errors.Is(err, autostart.ErrUnsupported) {
				u.fail(err)
			}
		})
		if on, err := u.autostart.IsEnabled(); err == nil {
			login.SetChecked(on)
		}
		items = append(items, login)
	}

	return container.NewVBox(items...)
}

func (u *UI) buildLog() fyne.CanvasObject {
	u.logView = widget.NewMultiLineEntry()
	u.logView.Wrapping = fyne.TextWrapOff
	// Read-only: this is for reading and copying, not editing.
	u.logView.Disable()

	scroll := container.NewVScroll(u.logView)
	scroll.SetMinSize(fyne.NewSize(0, 160))

	// The log lives in an accordion so the window stays near its intended
	// height. The copy button stays outside it and always visible: the support
	// flow is "paste me your log", and that must not be hidden behind a
	// disclosure triangle.
	copyBtn := widget.NewButton("Copy log", func() {
		u.app.Clipboard().SetContent(u.ctrl.LogText())
	})

	acc := widget.NewAccordion(widget.NewAccordionItem("Log", scroll))

	// The accordion already carries the "Log" heading, so the row above it is
	// just the button.
	return container.NewVBox(container.NewHBox(copyBtn), acc)
}

func (u *UI) buildFooter() fyne.CanvasObject {
	version := widget.NewLabel(u.ctrl.Version())

	u.updateLabel = widget.NewLabel("")
	u.updateLabel.Wrapping = fyne.TextWrapWord
	u.updateLabel.Hide()

	check := widget.NewButton("Check for updates", func() {
		dialog.ShowInformation("Updates", u.ctrl.CheckForUpdates(), u.win)
	})
	check.Importance = widget.LowImportance

	// The config path is worth showing, but not worth two wrapped lines of
	// window height, so it goes on the button that opens its folder.
	openCfg := widget.NewButton("Config file", func() { u.openConfigDir() })
	openCfg.Importance = widget.LowImportance

	return container.NewVBox(
		u.updateLabel,
		container.NewHBox(version, check, openCfg),
	)
}

func boldLabel(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.TextStyle = fyne.TextStyle{Bold: true}
	return l
}
