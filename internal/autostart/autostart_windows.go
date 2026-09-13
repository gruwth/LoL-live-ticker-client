//go:build windows

package autostart

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// runKey needs no elevation: HKCU is the current user's own hive.
const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

type windowsAutostart struct{}

func newPlatform() Interface { return windowsAutostart{} }

func (windowsAutostart) Supported() bool { return true }

func (windowsAutostart) IsEnabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()

	v, _, err := k.GetStringValue(Name)
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// A stale entry pointing at a binary that has moved is not "enabled" in
	// any useful sense, so report it as off and let the caller re-register.
	exe, err := os.Executable()
	if err != nil {
		return v != "", nil
	}
	return strings.Contains(strings.ToLower(v), strings.ToLower(exe)), nil
}

func (windowsAutostart) Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	// Quoted because Program Files has a space in it.
	return k.SetStringValue(Name, fmt.Sprintf("%q %s", exe, MinimizedFlag))
}

func (windowsAutostart) Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(Name); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}
