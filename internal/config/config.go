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

// Load reads path. A missing file is reported as ErrNoConfig.
func Load(path string) (Config, error) {
	c := Config{ShareActivePlayer: true}
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
