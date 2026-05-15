// Package handlers groups the HTTP handlers of the backend. Each handler
// receives a *Deps and returns a gin.HandlerFunc, which keeps wiring (in
// server.Build) decoupled from request logic and makes handlers testable
// with a stub Deps.
package handlers

import (
	"context"
	"database/sql"
	"sync"

	"streaming-poc/backend/internal/cache"
	"streaming-poc/backend/internal/config"
	"streaming-poc/backend/internal/process"
)

// Deps is the bag of long-lived dependencies that every handler needs. Kept
// as concrete types (not interfaces) because the POC has a single backend of
// each; introducing interfaces now would be premature abstraction.
type Deps struct {
	DB           *sql.DB
	Cache        *cache.Client
	Registry     *process.Registry
	Cfg          config.Config
	AppCtx       context.Context
	TranscodeSem chan struct{}
	BgWG         *sync.WaitGroup
}

// BgRun spawns fn in a background goroutine tracked by BgWG so server
// shutdown can wait for it to finish. fn receives the app-wide context, so
// long-running work cancels on SIGINT/SIGTERM.
func (d *Deps) BgRun(fn func(ctx context.Context)) {
	d.BgWG.Add(1)
	go func() {
		defer d.BgWG.Done()
		fn(d.AppCtx)
	}()
}
