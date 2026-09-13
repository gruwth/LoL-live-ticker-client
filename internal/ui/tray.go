//go:build !nogui

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// buildTray installs the tray icon and menu. Tray support is desktop-only and
// only reachable through this type assertion.
func (u *UI) buildTray() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return // no tray on this platform; the window is still fine
	}
	u.desk = desk
	u.refreshTray()
}

// refreshTray rebuilds the menu so the status line at the top stays current.
// Item ordering never changes, so muscle memory keeps working.
func (u *UI) refreshTray() {
	if u.desk == nil {
		return
	}

	status := fyne.NewMenuItem(labelForStatus(u.cur.Status), nil)
	status.Disabled = true

	open := fyne.NewMenuItem("Open", func() { u.Show() })

	copyLink := fyne.NewMenuItem("Copy share link", func() {
		u.app.Clipboard().SetContent(u.cur.ShareURL)
	})
	copyLink.Disabled = u.cur.ShareURL == ""

	openLive := fyne.NewMenuItem("Open live page", func() { u.openURL(u.cur.ShareURL) })
	openLive.Disabled = u.cur.ShareURL == ""

	// An available update shows up here as a labelled menu item, never as a
	// dialog that takes the screen away from a match in progress.
	updateLabel := "Check for updates"
	if st := u.ctrl.UpdateState(); st.Pending {
		updateLabel = "Update installs when your game ends"
	} else if st.Available {
		updateLabel = "Install update"
	}
	update := fyne.NewMenuItem(updateLabel, func() { u.ctrl.CheckForUpdates() })

	quit := fyne.NewMenuItem("Quit", func() { u.ctrl.Quit() })
	quit.IsQuit = true

	u.desk.SetSystemTrayIcon(iconForStatus(u.cur.Status))
	u.desk.SetSystemTrayMenu(fyne.NewMenu("LoL Live Ticker",
		status,
		fyne.NewMenuItemSeparator(),
		open,
		copyLink,
		openLive,
		fyne.NewMenuItemSeparator(),
		update,
		quit,
	))
}
