package handlers

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"streaming-poc/backend/internal/storage"
	"streaming-poc/backend/internal/transcode"
)

// PublishHook is called by MediaMTX when a publisher (OBS) connects on
// rtmp://.../live/<streamKey>. The hook URL carries path=live/<streamKey>.
func PublishHook(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := strings.TrimSpace(c.Query("path"))
		streamKey := strings.TrimPrefix(path, "live/")
		if streamKey == "" || streamKey == path {
			c.Status(http.StatusBadRequest)
			return
		}
		stream, err := storage.GetStreamByKey(c.Request.Context(), d.DB, streamKey)
		if err != nil {
			log.Printf("publish hook: unknown stream key %q: %v", streamKey, err)
			c.Status(http.StatusNotFound)
			return
		}
		d.BgRun(func(ctx context.Context) {
			transcode.StartLive(ctx, d.DB, d.Cache, d.Cfg.BackendRoot, stream, d.Registry)
		})
		c.Status(http.StatusOK)
	}
}

// UnpublishHook is called by MediaMTX when the publisher disconnects.
func UnpublishHook(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := strings.TrimSpace(c.Query("path"))
		streamKey := strings.TrimPrefix(path, "live/")
		if streamKey == "" || streamKey == path {
			c.Status(http.StatusBadRequest)
			return
		}
		d.Registry.Kill(streamKey)
		log.Printf("unpublish hook: stopped ffmpeg for stream key %q", streamKey)
		c.Status(http.StatusOK)
	}
}
