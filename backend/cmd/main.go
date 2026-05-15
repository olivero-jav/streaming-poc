package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"streaming-poc/backend/internal/cache"
	"streaming-poc/backend/internal/config"
	"streaming-poc/backend/internal/handlers"
	"streaming-poc/backend/internal/process"
	"streaming-poc/backend/internal/server"
	"streaming-poc/backend/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// appCtx is the lifetime of the process. SIGINT/SIGTERM cancels it, which
	// in turn cancels every ffmpeg started with exec.CommandContext(appCtx,...)
	// and every transcode/live ctx derived from it.
	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	initCtx, cancelInit := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelInit()

	log.Printf("Connecting to PostgreSQL at %s", cfg.DatabaseURL)
	db, err := storage.InitPostgres(initCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to initialize storage: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("failed to close storage: %v", closeErr)
		}
	}()

	cacheClient := cache.New(initCtx, cfg.RedisURL)
	defer func() {
		if closeErr := cacheClient.Close(); closeErr != nil {
			log.Printf("failed to close cache: %v", closeErr)
		}
	}()

	if err := storage.ResetStaleStreams(initCtx, db); err != nil {
		log.Printf("failed to reset stale streams: %v", err)
	}
	cacheClient.Del(initCtx, cache.KeyStreamList)

	log.Printf("upload size cap: %d bytes (~%d MB)", cfg.MaxUploadBytes, cfg.MaxUploadBytes>>20)

	var bgWG sync.WaitGroup
	deps := &handlers.Deps{
		DB:           db,
		Cache:        cacheClient,
		Registry:     process.NewRegistry(),
		Cfg:          cfg,
		AppCtx:       appCtx,
		TranscodeSem: make(chan struct{}, 4),
		BgWG:         &bgWG,
	}

	srv := &http.Server{Addr: cfg.Addr, Handler: server.Build(deps)}
	server.Run(appCtx, stop, srv, &bgWG)
}
