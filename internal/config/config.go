// Package config loads the config file and merges flags over it.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Server            string `json:"server"`
	Token             string `json:"token"`
	ShareActivePlayer bool   `json:"shareActivePlayer"`

	// LCU reads the League Client API for queue status and champ select. Off
	// by default: it is a wider surface than the in-game API and the user has
	// to ask for it.
	LCU bool `json:"lcu"`

	// Desktop-only settings. They are ignored by a headless build, but are
	// kept in the same file so one config describes the whole app.
	DiscordRPC       bool `json:"discordRpc"`
	StartMinimized   bool `json:"startMinimized"`
	AutoCheckUpdates bool `json:"autoCheckUpdates"`
}

// ErrNoConfig means there is nothing configured yet - first run.
var ErrNoConfig = errors.New("no config file")

// DefaultPath is %AppData%\lolticker\config.json on Windows,
// ~/.config/lolticker/config.json elsewhere.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lolticker", "config.json"), nil
}

// Defaults are what a config file gets before anything is read over it.
// Discord Rich Presence is off by default on purpose; the rest are the
// friendly choices for a first run.
func Defaults() Config {
	return Config{
		ShareActivePlayer: true,
		DiscordRPC:        false,
		AutoCheckUpdates:  true,
	}
}

// Load reads path. A missing file is reported as ErrNoConfig.
func Load(path string) (Config, error) {
	c := Defaults()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, ErrNoConfig
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, nil
}

// Save writes the config, creating the directory if needed. The file is
// written with 0600 because it holds a token.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	// Write to a sibling and rename, so a crash mid-write cannot leave the
	// user with a truncated config and no way back in.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Validate checks the fields the agent cannot run without.
func (c Config) Validate() error {
	if c.Server == "" {
		return errors.New("no server configured")
	}
	if c.Token == "" {
		return errors.New("no token configured")
	}
	return nil
}
