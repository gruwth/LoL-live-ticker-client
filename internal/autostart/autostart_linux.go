//go:build linux

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

type linuxAutostart struct{}

func newPlatform() Interface { return linuxAutostart{} }

func (linuxAutostart) Supported() bool { return true }

func (l linuxAutostart) path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "lolticker-agent.desktop"), nil
}

func (l linuxAutostart) IsEnabled() (bool, error) {
	p, err := l.path()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (l linuxAutostart) Enable() error {
	p, err := l.path()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Exec=%q %s
X-GNOME-Autostart-enabled=true
`, Name, exe, MinimizedFlag)
	return os.WriteFile(p, []byte(desktop), 0o644)
}

func (l linuxAutostart) Disable() error {
	p, err := l.path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
