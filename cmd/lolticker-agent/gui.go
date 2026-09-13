//go:build !nogui

package main

import (
	"context"
	"log/slog"

	"lolticker-agent/internal/agent"
	"lolticker-agent/internal/config"
	"lolticker-agent/internal/logtail"
	"lolticker-agent/internal/relay"
	"lolticker-agent/internal/singleton"
	"lolticker-agent/internal/ui"
)

// controller adapts the running app to what the window needs. It is the only
// place the UI and the core meet, which is what keeps internal/agent free of
// any Fyne import.
type controller struct {
	ag         *agent.Agent
	rl         *relay.Client
	logs       *logtail.Handler
	log        *slog.Logger
	configPath string
	version    string
	quit       func()

	cfg config.Config
}

func (c *controller) States() <-chan agent.State { return c.ag.States() }
func (c *controller) Config() config.Config      { return c.cfg }
func (c *controller) ConfigPath() string         { return c.configPath }
func (c *controller) LogText() string            { return c.logs.Text() }
func (c *controller) Version() string            { return c.version }
func (c *controller) Quit()                      { c.quit() }

// Save persists the settings and applies them live. Nothing here requires a
// restart, which is the whole point: a new user pastes a token and connects.
func (c *controller) Save(cfg config.Config) error {
	if err := config.Save(c.configPath, cfg); err != nil {
		return err
	}
	c.cfg = cfg
	c.logs.Redact(cfg.Token)
	c.ag.SetShareActivePlayer(cfg.ShareActivePlayer)
	c.rl.Reconfigure(cfg.Server, cfg.Token)
	return nil
}

// CheckForUpdates is wired in the updater step of the v3 scope. It logs for
// now so the button is honest about doing nothing yet rather than silently
// pretending.
func (c *controller) CheckForUpdates() {
	c.log.Info("update check requested, but the updater is not wired in yet")
}

// runFrontend for the desktop build: the window and the tray.
//
// The Fyne event loop must own the main goroutine, so this blocks. The single
// instance listener is served here too, which is what makes a second launch
// raise this window instead of starting a second agent.
func runFrontend(ctx context.Context, deps frontendDeps) {
	c := &controller{
		ag:         deps.agent,
		rl:         deps.relay,
		logs:       deps.logs,
		log:        deps.log,
		configPath: deps.configPath,
		version:    deps.version,
		quit:       deps.quit,
		cfg:        deps.config,
	}

	w := ui.New(c)
	go deps.lock.Serve(func() {
		deps.log.Info("another instance asked for the window")
		w.Show()
	})
	w.Run(ctx, deps.config.StartMinimized || deps.startMinimized)
}

var _ ui.Controller = (*controller)(nil)

// unused in this build, but keeps the import list identical across tags
var _ = singleton.Port

// guiBuild tells main whether a window exists to ask the user things.
const guiBuild = true
