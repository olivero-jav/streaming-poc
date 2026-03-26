package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log"
	"net/url"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"streaming-poc/backend/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func main() {
	backendRoot, err := resolveBackendRoot()
	if err != nil {
		log.Fatalf("failed to resolve backend root: %v", err)
	}

	initCtx, cancelInit := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelInit()

	dbPath := filepath.Join(backendRoot, "streaming.db")
	log.Printf("Using SQLite DB at %s", dbPath)

	db, err := storage.InitSQLite(initCtx, dbPath)
	if err != nil {
		log.Fatalf("failed to initialize storage: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("failed to close storage: %v", closeErr)
		}
	}()

	r := gin.Default()
	r.Use(corsMiddleware())

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
		uploadsDir := filepath.Join(backendRoot, "uploads")
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

		go processVideoAsync(db, backendRoot, video.ID, sourcePath)

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

		serveHLSVODFile(c, backendRoot, videoID, "index.m3u8")
	})

	r.GET("/hls/vod/:id/:segment", func(c *gin.Context) {
		videoID := strings.TrimSpace(c.Param("id"))
		segment := strings.TrimSpace(c.Param("segment"))
		if videoID == "" || segment == "" || strings.Contains(videoID, "..") || strings.Contains(segment, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
			return
		}

		serveHLSVODFile(c, backendRoot, videoID, segment)
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

func processVideoAsync(db *sql.DB, backendRoot, videoID, sourcePath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if _, err := storage.UpdateVideoStatus(ctx, db, videoID, "processing"); err != nil {
		log.Printf("failed to set video %s processing: %v", videoID, err)
		return
	}

	hlsDir := filepath.Join(backendRoot, "media", "hls", videoID)
	if err := os.MkdirAll(hlsDir, 0o755); err != nil {
		log.Printf("failed to create hls output directory for %s: %v", videoID, err)
		_, _ = storage.UpdateVideoStatus(ctx, db, videoID, "error")
		return
	}

	hlsPublicPath := filepath.ToSlash(filepath.Join("/hls/vod", videoID, "index.m3u8"))
	playlistPath := filepath.Join(hlsDir, "index.m3u8")

	var outputBuf bytes.Buffer
	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-y",
		"-i", sourcePath,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "event",
		playlistPath,
	)
	cmd.Stdout = &outputBuf
	cmd.Stderr = &outputBuf

	if err := cmd.Start(); err != nil {
		log.Printf("ffmpeg failed to start for %s: %v", videoID, err)
		_, _ = storage.UpdateVideoStatus(ctx, db, videoID, "error")
		return
	}

	// As soon as the first segment is on disk, expose the HLS path so the
	// client can start playing before the transcode finishes.
	go func() {
		firstSegment := filepath.Join(hlsDir, "index0.ts")
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := os.Stat(firstSegment); err == nil {
					if err := storage.SetHLSPath(ctx, db, videoID, hlsPublicPath); err != nil {
						log.Printf("failed to set hls_path for %s: %v", videoID, err)
					}
					return
				}
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		log.Printf("ffmpeg failed for %s: %v - %s", videoID, err, strings.TrimSpace(outputBuf.String()))
		_, _ = storage.UpdateVideoStatus(ctx, db, videoID, "error")
		return
	}

	if _, err := storage.MarkVideoReady(ctx, db, videoID, hlsPublicPath, 0); err != nil {
		log.Printf("failed to mark video %s ready: %v", videoID, err)
		_, _ = storage.UpdateVideoStatus(ctx, db, videoID, "error")
		return
	}
}

func serveHLSVODFile(c *gin.Context, backendRoot, videoID, fileName string) {
	baseDir := filepath.Join(backendRoot, "media", "hls", videoID)
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

func resolveBackendRoot() (string, error) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime caller lookup failed")
	}

	// cmd/main.go -> backend/
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..")), nil
}

func corsMiddleware() gin.HandlerFunc {
	allowedOrigins := loadAllowedOriginsFromEnv()
	allowNgrok := strings.EqualFold(strings.TrimSpace(os.Getenv("CORS_ALLOW_NGROK")), "true")

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if isAllowedOrigin(origin, allowedOrigins, allowNgrok) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}

		c.Header("Access-Control-Allow-Methods", "GET,POST,PATCH,HEAD,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,Range")
		c.Header("Access-Control-Expose-Headers", "Content-Length,Content-Range,Accept-Ranges")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func loadAllowedOriginsFromEnv() map[string]struct{} {
	origins := make(map[string]struct{})

	defaultOrigins := []string{
		"http://localhost:4200",
		"http://127.0.0.1:4200",
	}

	raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if raw == "" {
		for _, origin := range defaultOrigins {
			origins[origin] = struct{}{}
		}
		return origins
	}

	for _, entry := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(entry)
		if origin == "" {
			continue
		}
		origins[origin] = struct{}{}
	}

	return origins
}

func isAllowedOrigin(origin string, allowedOrigins map[string]struct{}, allowNgrok bool) bool {
	if origin == "" {
		return false
	}

	if _, ok := allowedOrigins[origin]; ok {
		return true
	}

	if !allowNgrok {
		return false
	}

	parsedOrigin, err := url.Parse(origin)
	if err != nil {
		return false
	}

	if parsedOrigin.Scheme != "https" {
		return false
	}

	host := strings.ToLower(parsedOrigin.Hostname())
	return strings.HasSuffix(host, ".ngrok-free.app") || strings.HasSuffix(host, ".ngrok.io")
}
