package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"streaming-poc/backend/internal/api"
	"streaming-poc/backend/internal/cache"
	"streaming-poc/backend/internal/storage"
	"streaming-poc/backend/internal/transcode"
)

func UploadVideo(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Force multipart parse up front so a MaxBytesReader trip surfaces as
		// 413 instead of being swallowed by Gin's PostForm (which would
		// return "" and make the handler look like "title is required").
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{
					"error":       fmt.Sprintf("upload exceeds the %d MB limit", d.Cfg.MaxUploadBytes>>20),
					"limit_bytes": d.Cfg.MaxUploadBytes,
				})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart form"})
			return
		}

		title := strings.TrimSpace(c.PostForm("title"))
		description := c.PostForm("description")
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
			return
		}

		fileHeader, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
			return
		}

		mimeType, err := api.DetectVideoMime(fileHeader)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read uploaded file"})
			return
		}
		if !api.AllowedVideoMimes[mimeType] {
			c.JSON(http.StatusUnsupportedMediaType, gin.H{
				"error":         "unsupported video format",
				"detected_mime": mimeType,
				"allowed":       []string{"video/mp4", "video/webm", "video/quicktime"},
			})
			return
		}

		videoID := uuid.NewString()
		uploadsDir := filepath.Join(d.Cfg.BackendRoot, "uploads")
		if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare uploads directory"})
			return
		}

		sourcePath := filepath.Join(uploadsDir, videoID+filepath.Ext(fileHeader.Filename))
		if err := c.SaveUploadedFile(fileHeader, sourcePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save uploaded file"})
			return
		}

		video, err := storage.CreateVideo(c.Request.Context(), d.DB, storage.CreateVideoInput{
			ID:          videoID,
			Title:       title,
			Description: description,
			SourcePath:  sourcePath,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create video"})
			return
		}
		d.Cache.Del(c.Request.Context(), cache.KeyVideoList)

		d.BgRun(func(ctx context.Context) {
			transcode.ProcessVideo(ctx, d.DB, d.Cache, d.Cfg.BackendRoot, video.ID, sourcePath, d.TranscodeSem)
		})

		c.JSON(http.StatusAccepted, video)
	}
}

func ListVideos(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		var videos []storage.Video
		if d.Cache.GetJSON(ctx, cache.KeyVideoList, &videos) {
			c.JSON(http.StatusOK, gin.H{"items": videos})
			return
		}

		videos, err := storage.ListVideos(ctx, d.DB)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list videos"})
			return
		}
		d.Cache.SetJSON(ctx, cache.KeyVideoList, videos, 30*time.Second)
		c.JSON(http.StatusOK, gin.H{"items": videos})
	}
}

func GetVideo(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")

		var cached storage.Video
		if d.Cache.GetJSON(ctx, cache.KeyVideo(id), &cached) {
			c.JSON(http.StatusOK, cached)
			return
		}

		video, err := storage.GetVideoByID(ctx, d.DB, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get video"})
			return
		}
		d.Cache.SetJSON(ctx, cache.KeyVideo(id), video, 60*time.Second)
		c.JSON(http.StatusOK, video)
	}
}

func UpdateVideoStatus(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Status string `json:"status"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json payload"})
			return
		}

		status := strings.TrimSpace(req.Status)
		if !isValidVideoStatus(status) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status; allowed: pending, processing, ready, error"})
			return
		}

		id := c.Param("id")
		video, err := storage.UpdateVideoStatus(c.Request.Context(), d.DB, id, status)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update video status"})
			return
		}
		d.Cache.Del(c.Request.Context(), cache.KeyVideoList, cache.KeyVideo(id))
		c.JSON(http.StatusOK, video)
	}
}

func isValidVideoStatus(status string) bool {
	switch status {
	case "pending", "processing", "ready", "error":
		return true
	default:
		return false
	}
}
