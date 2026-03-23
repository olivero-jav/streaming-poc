package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gin-gonic/gin"
	"streaming-poc/backend/internal/storage"
)

func main() {
	initCtx, cancelInit := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelInit()

	db, err := storage.InitSQLite(initCtx, "./streaming.db")
	if err != nil {
		log.Fatalf("failed to initialize storage: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("failed to close storage: %v", closeErr)
		}
	}()

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	r.POST("/videos", func(c *gin.Context) {
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

		videoID := uuid.NewString()
		uploadsDir := filepath.Join(".", "uploads")
		if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare uploads directory"})
			return
		}

		sourcePath := filepath.Join(uploadsDir, videoID+filepath.Ext(fileHeader.Filename))
		if err := c.SaveUploadedFile(fileHeader, sourcePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save uploaded file"})
			return
		}

		video, err := storage.CreateVideo(c.Request.Context(), db, storage.CreateVideoInput{
			ID:          videoID,
			Title:       title,
			Description: description,
			SourcePath:  sourcePath,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create video"})
			return
		}

		go processVideoAsync(db, video.ID, sourcePath)

		c.JSON(http.StatusAccepted, video)
	})

	r.GET("/videos", func(c *gin.Context) {
		videos, err := storage.ListVideos(c.Request.Context(), db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list videos"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"items": videos})
	})

	r.GET("/videos/:id", func(c *gin.Context) {
		video, err := storage.GetVideoByID(c.Request.Context(), db, c.Param("id"))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get video"})
			return
		}

		c.JSON(http.StatusOK, video)
	})

	r.GET("/hls/vod/:id/index.m3u8", func(c *gin.Context) {
		videoID := strings.TrimSpace(c.Param("id"))
		if videoID == "" || strings.Contains(videoID, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid video id"})
			return
		}

		serveHLSVODFile(c, videoID, "index.m3u8")
	})

	r.GET("/hls/vod/:id/:segment", func(c *gin.Context) {
		videoID := strings.TrimSpace(c.Param("id"))
		segment := strings.TrimSpace(c.Param("segment"))
		if videoID == "" || segment == "" || strings.Contains(videoID, "..") || strings.Contains(segment, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
			return
		}

		serveHLSVODFile(c, videoID, segment)
	})

	r.PATCH("/videos/:id/status", func(c *gin.Context) {
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

		video, err := storage.UpdateVideoStatus(c.Request.Context(), db, c.Param("id"), status)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update video status"})
			return
		}

		c.JSON(http.StatusOK, video)
	})

	log.Println("Starting HTTP server on http://localhost:8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server failed: %v", err)
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

func processVideoAsync(db *sql.DB, videoID, sourcePath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if _, err := storage.UpdateVideoStatus(ctx, db, videoID, "processing"); err != nil {
		log.Printf("failed to set video %s processing: %v", videoID, err)
		return
	}

	hlsDir := filepath.Join(".", "media", "hls", videoID)
	if err := os.MkdirAll(hlsDir, 0o755); err != nil {
		log.Printf("failed to create hls output directory for %s: %v", videoID, err)
		_, _ = storage.UpdateVideoStatus(ctx, db, videoID, "error")
		return
	}

	playlistPath := filepath.Join(hlsDir, "index.m3u8")
	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-y",
		"-i", sourcePath,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		playlistPath,
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("ffmpeg failed for %s: %v - %s", videoID, err, strings.TrimSpace(string(output)))
		_, _ = storage.UpdateVideoStatus(ctx, db, videoID, "error")
		return
	}

	hlsPublicPath := filepath.ToSlash(filepath.Join("/hls/vod", videoID, "index.m3u8"))
	if _, err := storage.MarkVideoReady(ctx, db, videoID, hlsPublicPath, 0); err != nil {
		log.Printf("failed to mark video %s ready: %v", videoID, err)
		_, _ = storage.UpdateVideoStatus(ctx, db, videoID, "error")
		return
	}
}

func serveHLSVODFile(c *gin.Context, videoID, fileName string) {
	baseDir := filepath.Join(".", "media", "hls", videoID)
	targetPath := filepath.Clean(filepath.Join(baseDir, fileName))
	if !strings.HasPrefix(targetPath, filepath.Clean(baseDir)+string(filepath.Separator)) && targetPath != filepath.Clean(baseDir) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file path"})
		return
	}

	switch filepath.Ext(fileName) {
	case ".m3u8":
		c.Header("Content-Type", "application/vnd.apple.mpegurl")
	case ".ts":
		c.Header("Content-Type", "video/mp2t")
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported file type"})
		return
	}

	c.File(targetPath)
}
