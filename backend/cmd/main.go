package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"streaming-poc/backend/internal/cache"
	"streaming-poc/backend/internal/process"
	"streaming-poc/backend/internal/storage"

	"github.com/gin-contrib/gzip"
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

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://streaming_user:streaming_pass@localhost:5432/streaming?sslmode=disable"
	}
	log.Printf("Connecting to PostgreSQL at %s", databaseURL)

	db, err := storage.InitPostgres(initCtx, databaseURL)
	if err != nil {
		log.Fatalf("failed to initialize storage: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("failed to close storage: %v", closeErr)
		}
	}()

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	cacheClient := cache.New(initCtx, redisURL)
	defer func() {
		if closeErr := cacheClient.Close(); closeErr != nil {
			log.Printf("failed to close cache: %v", closeErr)
		}
	}()

	if err := storage.ResetStaleStreams(initCtx, db); err != nil {
		log.Printf("failed to reset stale streams: %v", err)
	}
	cacheClient.Del(initCtx, cache.KeyStreamList)

	registry := process.NewRegistry()

	transcodeSem := make(chan struct{}, 4)

	r := gin.Default()
	r.Use(corsMiddleware())
	r.Use(gzip.Gzip(gzip.DefaultCompression,
		gzip.WithExcludedExtensions([]string{".ts", ".m4s", ".mp4", ".aac"})))

	backendCommit := os.Getenv("GIT_COMMIT")
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":   "ok",
			"redis_up": cacheClient.IsConnected(),
			"commit":   backendCommit,
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
		cacheClient.Del(c.Request.Context(), cache.KeyVideoList)

		go processVideoAsync(db, cacheClient, backendRoot, video.ID, sourcePath, transcodeSem)

		c.JSON(http.StatusAccepted, video)
	})

	r.GET("/videos", func(c *gin.Context) {
		ctx := c.Request.Context()
		var videos []storage.Video
		if cacheClient.GetJSON(ctx, cache.KeyVideoList, &videos) {
			c.JSON(http.StatusOK, gin.H{"items": videos})
			return
		}

		videos, err := storage.ListVideos(ctx, db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list videos"})
			return
		}
		cacheClient.SetJSON(ctx, cache.KeyVideoList, videos, 30*time.Second)

		c.JSON(http.StatusOK, gin.H{"items": videos})
	})

	r.GET("/videos/:id", func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")
		var video storage.Video
		if cacheClient.GetJSON(ctx, cache.KeyVideo(id), &video) {
			c.JSON(http.StatusOK, video)
			return
		}

		video, err := storage.GetVideoByID(ctx, db, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get video"})
			return
		}
		cacheClient.SetJSON(ctx, cache.KeyVideo(id), video, 60*time.Second)

		c.JSON(http.StatusOK, video)
	})

	r.GET("/hls/vod/:id/index.m3u8", func(c *gin.Context) {
		videoID := strings.TrimSpace(c.Param("id"))
		if videoID == "" || strings.Contains(videoID, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid video id"})
			return
		}
		serveHLSFile(c, filepath.Join(backendRoot, "media", "hls", videoID), "index.m3u8")
	})

	r.GET("/hls/vod/:id/:segment", func(c *gin.Context) {
		videoID := strings.TrimSpace(c.Param("id"))
		segment := strings.TrimSpace(c.Param("segment"))
		if videoID == "" || segment == "" || strings.Contains(videoID, "..") || strings.Contains(segment, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
			return
		}
		serveHLSFile(c, filepath.Join(backendRoot, "media", "hls", videoID), segment)
	})

	// Streams
	r.POST("/streams", func(c *gin.Context) {
		var req struct {
			Title string `json:"title"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json payload"})
			return
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
			return
		}
		stream, err := storage.CreateStream(c.Request.Context(), db, storage.CreateStreamInput{
			ID:        uuid.NewString(),
			Title:     title,
			StreamKey: uuid.NewString(),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create stream"})
			return
		}
		cacheClient.Del(c.Request.Context(), cache.KeyStreamList)
		c.JSON(http.StatusCreated, stream)
	})

	r.GET("/streams", func(c *gin.Context) {
		ctx := c.Request.Context()
		var streams []storage.Stream
		if cacheClient.GetJSON(ctx, cache.KeyStreamList, &streams) {
			c.JSON(http.StatusOK, gin.H{"items": streams})
			return
		}

		streams, err := storage.ListStreams(ctx, db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list streams"})
			return
		}
		cacheClient.SetJSON(ctx, cache.KeyStreamList, streams, 30*time.Second)
		c.JSON(http.StatusOK, gin.H{"items": streams})
	})

	r.GET("/streams/:id", func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")
		var stream storage.Stream
		if cacheClient.GetJSON(ctx, cache.KeyStream(id), &stream) {
			c.JSON(http.StatusOK, stream)
			return
		}

		stream, err := storage.GetStreamByID(ctx, db, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "stream not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stream"})
			return
		}
		cacheClient.SetJSON(ctx, cache.KeyStream(id), stream, 60*time.Second)
		c.JSON(http.StatusOK, stream)
	})

	// MediaMTX hooks — called from localhost by MediaMTX when OBS connects/disconnects.
	r.POST("/internal/hooks/publish", func(c *gin.Context) {
		path := strings.TrimSpace(c.Query("path"))
		streamKey := strings.TrimPrefix(path, "live/")
		if streamKey == "" || streamKey == path {
			c.Status(http.StatusBadRequest)
			return
		}
		stream, err := storage.GetStreamByKey(c.Request.Context(), db, streamKey)
		if err != nil {
			log.Printf("publish hook: unknown stream key %q: %v", streamKey, err)
			c.Status(http.StatusNotFound)
			return
		}
		go startLiveStream(db, cacheClient, backendRoot, stream, registry)
		c.Status(http.StatusOK)
	})

	r.POST("/internal/hooks/unpublish", func(c *gin.Context) {
		path := strings.TrimSpace(c.Query("path"))
		streamKey := strings.TrimPrefix(path, "live/")
		if streamKey == "" || streamKey == path {
			c.Status(http.StatusBadRequest)
			return
		}
		registry.Kill(streamKey)
		log.Printf("unpublish hook: stopped ffmpeg for stream key %q", streamKey)
		c.Status(http.StatusOK)
	})

	// Live HLS
	r.GET("/hls/live/:id/index.m3u8", func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" || strings.Contains(id, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid stream id"})
			return
		}
		serveHLSFile(c, filepath.Join(backendRoot, "media", "live", id), "index.m3u8")
	})

	r.GET("/hls/live/:id/:segment", func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		segment := strings.TrimSpace(c.Param("segment"))
		if id == "" || segment == "" || strings.Contains(id, "..") || strings.Contains(segment, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
			return
		}
		serveHLSFile(c, filepath.Join(backendRoot, "media", "live", id), segment)
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

		id := c.Param("id")
		video, err := storage.UpdateVideoStatus(c.Request.Context(), db, id, status)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update video status"})
			return
		}
		cacheClient.Del(c.Request.Context(), cache.KeyVideoList, cache.KeyVideo(id))

		c.JSON(http.StatusOK, video)
	})

	distDir := filepath.Join(backendRoot, "..", "frontend", "streaming-frontend", "dist", "streaming-frontend", "browser")
	if _, err := os.Stat(distDir); err == nil {
		r.NoRoute(func(c *gin.Context) {
			urlPath := c.Request.URL.Path
			filePath := filepath.Clean(filepath.Join(distDir, filepath.FromSlash(urlPath)))
			cleanDist := filepath.Clean(distDir)
			if filePath != cleanDist && !strings.HasPrefix(filePath, cleanDist+string(filepath.Separator)) {
				c.Status(http.StatusForbidden)
				return
			}
			if info, statErr := os.Stat(filePath); statErr == nil && !info.IsDir() {
				if strings.ToLower(filepath.Ext(filePath)) == ".html" {
					c.Header("Cache-Control", "no-cache")
				} else {
					c.Header("Cache-Control", "max-age=3600, public")
				}
				c.File(filePath)
				return
			}
			c.Header("Cache-Control", "no-cache")
			c.File(filepath.Join(distDir, "index.html"))
		})
		log.Printf("Serving Angular frontend from %s", distDir)
	}

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

func processVideoAsync(db *sql.DB, cacheClient *cache.Client, backendRoot, videoID, sourcePath string, sem chan struct{}) {
	sem <- struct{}{}
	defer func() { <-sem }()

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
					cacheClient.Del(ctx, cache.KeyVideoList, cache.KeyVideo(videoID))
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
	cacheClient.Del(ctx, cache.KeyVideoList, cache.KeyVideo(videoID))
}

func serveHLSFile(c *gin.Context, baseDir, fileName string) {
	targetPath := filepath.Clean(filepath.Join(baseDir, fileName))
	if !strings.HasPrefix(targetPath, filepath.Clean(baseDir)+string(filepath.Separator)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file path"})
		return
	}

	c.Header("Accept-Ranges", "bytes")
	switch filepath.Ext(fileName) {
	case ".m3u8":
		c.Header("Content-Type", "application/vnd.apple.mpegurl")
		c.Header("Cache-Control", "max-age=2, public")
	case ".ts":
		c.Header("Content-Type", "video/mp2t")
		c.Header("Cache-Control", "max-age=31536000, immutable")
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported file type"})
		return
	}

	c.File(targetPath)
}

func startLiveStream(db *sql.DB, cacheClient *cache.Client, backendRoot string, stream storage.Stream, registry *process.Registry) {
	ctx := context.Background()

	hlsDir := filepath.Join(backendRoot, "media", "live", stream.ID)
	if err := os.MkdirAll(hlsDir, 0o755); err != nil {
		log.Printf("failed to create live hls dir for stream %s: %v", stream.ID, err)
		return
	}

	hlsPublicPath := "/hls/live/" + stream.ID + "/index.m3u8"
	playlistPath := filepath.Join(hlsDir, "index.m3u8")

	cmd := exec.Command(
		"ffmpeg",
		"-i", "rtmp://localhost:1935/live/"+stream.StreamKey,
		"-c:v", "copy",
		"-c:a", "aac",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "event",
		playlistPath,
	)
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("failed to start ffmpeg for stream %s: %v", stream.ID, err)
		return
	}

	registry.Register(stream.StreamKey, cmd)

	// Poll for the first segment, then expose the stream for playback.
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.After(30 * time.Second)
		for {
			select {
			case <-deadline:
				log.Printf("timed out waiting for first live segment of stream %s", stream.ID)
				return
			case <-ticker.C:
				if _, err := os.Stat(filepath.Join(hlsDir, "index0.ts")); err == nil {
					if err := storage.MarkStreamLive(ctx, db, stream.ID, hlsPublicPath); err != nil {
						log.Printf("failed to mark stream %s live: %v", stream.ID, err)
					}
					cacheClient.Del(ctx, cache.KeyStreamList, cache.KeyStream(stream.ID))
					return
				}
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		log.Printf("ffmpeg exited for stream %s: %v", stream.ID, err)
	}

	registry.Kill(stream.StreamKey)
	if err := storage.MarkStreamEnded(ctx, db, stream.ID); err != nil {
		log.Printf("failed to mark stream %s ended: %v", stream.ID, err)
	}
	cacheClient.Del(ctx, cache.KeyStreamList, cache.KeyStream(stream.ID))

	go promoteStreamToVOD(db, cacheClient, backendRoot, stream)
}

// promoteStreamToVOD creates a ready VOD entry from the HLS files produced
// during a live stream session. The files are served in place — no copy needed.
func promoteStreamToVOD(db *sql.DB, cacheClient *cache.Client, backendRoot string, stream storage.Stream) {
	playlistPath := filepath.Join(backendRoot, "media", "live", stream.ID, "index.m3u8")
	if _, err := os.Stat(playlistPath); err != nil {
		log.Printf("promote stream %s: no HLS playlist found, skipping VOD creation", stream.ID)
		return
	}

	if err := finalizeHLSPlaylist(playlistPath); err != nil {
		log.Printf("promote stream %s: failed to finalize playlist: %v", stream.ID, err)
		// non-fatal: proceed anyway, playback may still work
	}

	hlsPublicPath := "/hls/live/" + stream.ID + "/index.m3u8"
	ctx := context.Background()
	video, err := storage.CreateVideoFromStream(ctx, db, uuid.NewString(), stream.Title, hlsPublicPath)
	if err != nil {
		log.Printf("promote stream %s: failed to create VOD record: %v", stream.ID, err)
		return
	}
	cacheClient.Del(ctx, cache.KeyVideoList)
	log.Printf("stream %s promoted to VOD %s (%s)", stream.ID, video.ID, stream.Title)
}

// finalizeHLSPlaylist appends #EXT-X-ENDLIST to the playlist if it is missing.
// FFmpeg may skip this tag when it is killed rather than shut down cleanly.
func finalizeHLSPlaylist(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read playlist: %w", err)
	}
	content := strings.TrimRight(string(data), "\r\n")
	if strings.HasSuffix(content, "#EXT-X-ENDLIST") {
		return nil
	}
	return os.WriteFile(path, []byte(content+"\n#EXT-X-ENDLIST\n"), 0o644)
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
