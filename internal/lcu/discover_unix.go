//go:build !windows

package lcu

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// processCommandLine greps the process table for the client.
func processCommandLine() (string, error) {
	out, err := exec.Command("ps", "-Ao", "args").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "LeagueClientUx") {
			return line, nil
		}
	}
	return "", ErrNotRunning
}

func installDirs() []string {
	dirs := []string{"/Applications/League of Legends.app/Contents/LoL"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, "Applications", "League of Legends.app", "Contents", "LoL"),
			filepath.Join(home, ".wine", "drive_c", "Riot Games", "League of Legends"),
		)
	}
	return dirs
}
