// Package server wires the HTTP layer (middleware + routes) and owns the
// graceful-shutdown lifecycle.
package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"

	"streaming-poc/backend/internal/api"
	"streaming-poc/backend/internal/handlers"
)

// Build wires every middleware and route into a gin.Engine ready to serve.
// All HTTP behavior of the backend lives in here; main() only calls Build
// followed by Run.
func Build(d *handlers.Deps) *gin.Engine {
	r := gin.Default()
	r.Use(api.CORS(d.Cfg.CORSAllowedOrigins, d.Cfg.CORSAllowNgrok))
	r.Use(gzip.Gzip(gzip.DefaultCompression,
		gzip.WithExcludedExtensions([]string{".ts", ".m4s", ".mp4", ".aac"})))
	r.Use(api.SecurityHeaders())

	r.GET("/health", handlers.Health(d))

	r.POST("/videos", api.LimitUploadSize(d.Cfg.MaxUploadBytes), handlers.UploadVideo(d))
	r.GET("/videos", handlers.ListVideos(d))
	r.GET("/videos/:id", handlers.GetVideo(d))
	r.PATCH("/videos/:id/status", handlers.UpdateVideoStatus(d))

	r.GET("/hls/vod/:id/index.m3u8", handlers.ServeHLSPlaylist(d, "hls", "video id"))
	r.GET("/hls/vod/:id/:segment", handlers.ServeHLSSegment(d, "hls"))

	r.POST("/streams", handlers.CreateStream(d))
	r.GET("/streams", handlers.ListStreams(d))
	r.GET("/streams/:id", handlers.GetStream(d))

	// MediaMTX hooks — called from localhost by MediaMTX when OBS connects/disconnects.
	r.POST("/internal/hooks/publish", handlers.PublishHook(d))
	r.POST("/internal/hooks/unpublish", handlers.UnpublishHook(d))

	r.GET("/hls/live/:id/index.m3u8", handlers.ServeHLSPlaylist(d, "live", "stream id"))
	r.GET("/hls/live/:id/:segment", handlers.ServeHLSSegment(d, "live"))

	distDir := filepath.Join(d.Cfg.BackendRoot, "..", "frontend", "streaming-frontend", "dist", "streaming-frontend", "browser")
	if fallback := handlers.SPAFallback(distDir); fallback != nil {
		r.NoRoute(fallback)
	}

	return r
}

// Run starts srv and blocks until appCtx is cancelled or srv exits on its own.
// On shutdown it Shutdown()s the server (10s budget) then waits for tracked
// background goroutines to drain (15s budget). stop is invoked if the server
// fails on its own, so background work cancels too.
func Run(appCtx context.Context, stop context.CancelFunc, srv *http.Server, bgWG *sync.WaitGroup) {
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Starting HTTP server on http://localhost%s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-appCtx.Done():
		log.Println("shutdown signal received, draining...")
	case err := <-serverErr:
		if err != nil {
			log.Printf("server failed: %v", err)
		}
		stop()
	}

	// Stop accepting new HTTP requests but let in-flight ones finish.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}

	// Drain background workers (transcodes, live sessions). Their ffmpeg
	// processes are already cancellable via appCtx; this just waits for the
	// goroutines themselves to finish their cleanup.
	drained := make(chan struct{})
	go func() {
		bgWG.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		log.Println("background workers drained")
	case <-time.After(15 * time.Second):
		log.Println("background workers did not drain in time, exiting anyway")
	}
}
