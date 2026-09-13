// Package autostart turns "start on login" on and off, per platform, behind
// one interface.
package autostart

import "errors"

// ErrUnsupported means this platform has no implementation yet. Callers should
// hide the option rather than show a control that cannot work.
var ErrUnsupported = errors.New("start on login is not supported on this platform")

// Name is the label the entry is registered under.
const Name = "LoL Live Ticker Agent"

// Interface is what the UI depends on.
type Interface interface {
	Supported() bool
	IsEnabled() (bool, error)
	Enable() error
	Disable() error
}

// New returns the implementation for the running platform.
func New() Interface { return newPlatform() }

// MinimizedFlag is appended to the registered command so an auto-started agent
// comes up in the tray instead of throwing a window at someone who just logged
// in.
const MinimizedFlag = "--minimized"
