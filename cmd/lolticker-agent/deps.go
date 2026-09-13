package main

import (
	"context"
	"log/slog"

	"lolticker-agent/internal/agent"
	"lolticker-agent/internal/config"
	"lolticker-agent/internal/logtail"
	"lolticker-agent/internal/relay"
	"lolticker-agent/internal/singleton"
)

// frontendDeps is everything a front end might need, assembled by main. The
// GUI build uses all of it; the headless build uses three fields. Passing one
// struct keeps the two runFrontend signatures identical, which is what stops
// the build tags drifting apart.
type frontendDeps struct {
	agent          *agent.Agent
	relay          *relay.Client
	lock           *singleton.Lock
	logs           *logtail.Handler
	log            *slog.Logger
	config         config.Config
	configPath     string
	version        string
	startMinimized bool
	quit           func()
}

var _ = context.Background
