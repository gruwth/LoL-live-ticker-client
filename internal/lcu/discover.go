// Package lcu reads two endpoints from the local League Client. It is off by
// default and does nothing until the user turns it on.
//
// The endpoint allowlist in client.go is the auditable claim this package
// exists to keep: the LCU exposes chat, friends and match history, and this
// agent reads the game phase and the champ select session, full stop.
package lcu

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ErrNotRunning means the League client is not up. It is the normal state for
// most of the day and is not worth logging.
var ErrNotRunning = errors.New("league client is not running")

// Credentials are the port and token the client was started with. Both change
// on every client restart, which is why nothing may cache them for the
// lifetime of the session.
type Credentials struct {
	Port  int
	Token string
}

var (
	portRe  = regexp.MustCompile(`--app-port=([0-9]+)`)
	tokenRe = regexp.MustCompile(`--remoting-auth-token=([\w-]+)`)
)

// Discover finds the running client's port and token.
//
// The process command line is the primary source because it carries both
// values directly, where the lockfile first requires knowing an install path
// that varies per machine. Note that the command line is read through the
// operating system's own process table, never by reading another process's
// memory: that is what Vanguard blocks, and this must never start doing it.
func Discover() (Credentials, error) {
	cmdline, err := processCommandLine()
	if err == nil && cmdline != "" {
		if c, ok := parseCommandLine(cmdline); ok {
			return c, nil
		}
	}
	// Fall back only if the process scan yielded nothing usable.
	if c, ok := fromLockfile(); ok {
		return c, nil
	}
	if err != nil && !errors.Is(err, ErrNotRunning) {
		return Credentials{}, fmt.Errorf("finding the league client: %w", err)
	}
	return Credentials{}, ErrNotRunning
}

func parseCommandLine(s string) (Credentials, bool) {
	pm := portRe.FindStringSubmatch(s)
	tm := tokenRe.FindStringSubmatch(s)
	if pm == nil || tm == nil {
		return Credentials{}, false
	}
	port, err := strconv.Atoi(pm[1])
	if err != nil || port == 0 {
		return Credentials{}, false
	}
	return Credentials{Port: port, Token: tm[1]}, true
}

// parseLockfile reads the colon-separated form: name:pid:port:password:protocol
func parseLockfile(s string) (Credentials, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 4 {
		return Credentials{}, false
	}
	port, err := strconv.Atoi(parts[2])
	if err != nil || port == 0 || parts[3] == "" {
		return Credentials{}, false
	}
	return Credentials{Port: port, Token: parts[3]}, true
}

func fromLockfile() (Credentials, bool) {
	for _, p := range lockfilePaths() {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if c, ok := parseLockfile(string(b)); ok {
			return c, true
		}
	}
	return Credentials{}, false
}

// lockfilePaths lists the usual install locations. This is a fallback for the
// case where the process scan fails, so a short list of common paths is the
// right amount of effort.
func lockfilePaths() []string {
	var out []string
	for _, dir := range installDirs() {
		out = append(out, filepath.Join(dir, "lockfile"))
	}
	return out
}
