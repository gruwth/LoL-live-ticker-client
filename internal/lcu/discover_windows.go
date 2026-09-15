//go:build windows

package lcu

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// processCommandLine asks Windows for the client's command line.
//
// CIM is used rather than the Win32 API because the only way to read another
// process's command line through the API is to walk its PEB, which is process
// memory reading -- exactly what Vanguard blocks and what this agent promises
// not to do. The OS hands the same string over voluntarily here.
func processCommandLine() (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		`Get-CimInstance Win32_Process -Filter "Name='LeagueClientUx.exe'" | `+
			`Select-Object -ExpandProperty CommandLine`)
	// Without this a GUI build flashes a console window every time it looks.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "", ErrNotRunning
	}
	return s, nil
}

func installDirs() []string {
	dirs := []string{
		`C:\Riot Games\League of Legends`,
		`C:\Program Files\Riot Games\League of Legends`,
		`C:\Program Files (x86)\Riot Games\League of Legends`,
	}
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		dirs = append(dirs, filepath.Join(d, "Riot Games", "League of Legends"))
	}
	return dirs
}
