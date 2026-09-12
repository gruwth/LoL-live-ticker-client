// Command lolticker-agent reads the local League of Legends live client API
// and streams a lean version of it to one configured relay. It does nothing
// else: no file access beyond its own config, no other network calls,
// no telemetry.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lolticker-agent/internal/agent"
	"lolticker-agent/internal/config"
	"lolticker-agent/internal/relay"
	"lolticker-agent/internal/riot"
	"lolticker-agent/internal/transform"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lolticker-agent:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath  = flag.String("config", "", "alternate config file")
		server      = flag.String("server", "", "relay WebSocket URL")
		token       = flag.String("token", "", "agent token (env LOLTICKER_TOKEN also accepted)")
		noActive    = flag.Bool("no-active", false, "do not send gold/stats/abilities of the local player")
		dump        = flag.String("dump", "", "record raw allgamedata to a JSONL file instead of sending")
		once        = flag.Bool("once", false, "single poll, print lean snapshot to stdout, exit")
		verbose     = flag.Bool("verbose", false, "debug logging")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("lolticker-agent", version)
		return nil
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rc := riot.NewClient()

	// --once and --dump only read the game, so they need no config at all.
	if *once {
		return runOnce(ctx, rc, !*noActive)
	}
	if *dump != "" {
		return runDump(ctx, rc, *dump, log)
	}

	cfg, err := loadConfig(*configPath, *server, *token, *noActive)
	if err != nil {
		return err
	}

	rl := relay.New(cfg.Server, cfg.Token, version, log)
	ag := agent.New(rc, rl, cfg.ShareActivePlayer, log)

	log.Info("starting", "version", version, "server", cfg.Server, "shareActivePlayer", cfg.ShareActivePlayer)

	errs := make(chan error, 2)
	go func() { errs <- rl.Run(ctx) }()
	go func() { errs <- ag.Run(ctx) }()

	err = <-errs
	stop()
	if errors.Is(err, relay.ErrUnauthorized) {
		return fmt.Errorf("the relay rejected this token - check `token` in your config file (%v)", err)
	}
	return err
}

func loadConfig(path, server, token string, noActive bool) (config.Config, error) {
	var err error
	if path == "" {
		if path, err = config.DefaultPath(); err != nil {
			return config.Config{}, err
		}
	}

	cfg, loadErr := config.Load(path)
	if loadErr != nil && !errors.Is(loadErr, config.ErrNoConfig) {
		return cfg, loadErr
	}

	// Flags and env override the file.
	if server != "" {
		cfg.Server = server
	}
	if token != "" {
		cfg.Token = token
	} else if env := os.Getenv("LOLTICKER_TOKEN"); env != "" && cfg.Token == "" {
		cfg.Token = env
	}
	if noActive {
		cfg.ShareActivePlayer = false
	}

	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("%w\n\n%s", err, setupHelp(path))
	}
	return cfg, nil
}

func setupHelp(path string) string {
	return fmt.Sprintf(`Nothing is configured yet.

1. Open the ticker site, sign in and copy your agent token.
2. Create %s with:

{
  "server": "wss://lol.erxt.dev/ws/agent",
  "token": "lt_your_token_here",
  "shareActivePlayer": true
}

3. Start the agent again, with League running or not.

Flags override the file: --server, --token (or LOLTICKER_TOKEN), --no-active.`, path)
}

// runOnce polls once and prints the exact snapshot the relay would receive.
func runOnce(ctx context.Context, rc *riot.Client, includeActive bool) error {
	data, err := rc.AllGameData(ctx)
	if err != nil {
		if riot.IsGameNotRunning(err) {
			return errors.New("no live game running")
		}
		return err
	}
	snap := transform.Snapshot(data, includeActive)
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// runDump records raw allgamedata to a JSONL file so bug reports can ship a
// recording. Nothing is sent anywhere.
func runDump(ctx context.Context, rc *riot.Client, path string, log *slog.Logger) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	log.Info("recording raw allgamedata", "path", path)

	t := time.NewTicker(time.Second)
	defer t.Stop()
	var n int
	for {
		select {
		case <-ctx.Done():
			log.Info("stopped recording", "lines", n)
			return nil
		case <-t.C:
		}
		body, err := rc.Raw(ctx, "/allgamedata")
		if err != nil {
			if !riot.IsGameNotRunning(err) {
				log.Warn("poll failed", "err", err)
			}
			continue
		}
		if _, err := f.Write(append(compact(body), '\n')); err != nil {
			return err
		}
		n++
	}
}

// compact strips the whitespace Riot puts in the payload so one poll is one line.
func compact(b []byte) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		return b
	}
	return buf.Bytes()
}
