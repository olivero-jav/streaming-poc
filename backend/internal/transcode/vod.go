// Package transcode owns the ffmpeg-backed background work: VOD transcoding
// and live RTMP ingestion. The HTTP layer dispatches into here from background
// goroutines and never blocks on it directly.
package transcode

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"streaming-poc/backend/internal/cache"
	"streaming-poc/backend/internal/storage"
)

// ProcessVideo transcodes the source file at sourcePath into HLS under
// {backendRoot}/media/hls/{videoID}/ using ffmpeg. Blocking by design; intended
// to run from a background goroutine. The ffmpeg invocation is bound to
// appCtx, so shutdown propagates as a kill signal to the running process.
//
// sem is a counting semaphore that caps concurrent transcodes across the app.
func ProcessVideo(appCtx context.Context, db *sql.DB, cacheClient *cache.Client, backendRoot, videoID, sourcePath string, sem chan struct{}) {
	select {
	case sem <- struct{}{}:
	case <-appCtx.Done():
		return
	}
	defer func() { <-sem }()

	runCtx, cancel := context.WithTimeout(appCtx, 30*time.Minute)
	defer cancel()

	if _, err := storage.UpdateVideoStatus(runCtx, db, videoID, "processing"); err != nil {
		log.Printf("failed to set video %s processing: %v", videoID, err)
		return
	}

	hlsDir := filepath.Join(backendRoot, "media", "hls", videoID)
	if err := os.MkdirAll(hlsDir, 0o755); err != nil {
		log.Printf("failed to create hls output directory for %s: %v", videoID, err)
		markVideoError(db, videoID)
		return
	}

	hlsPublicPath := filepath.ToSlash(filepath.Join("/hls/vod", videoID, "index.m3u8"))
	playlistPath := filepath.Join(hlsDir, "index.m3u8")

	output := &lockedBuffer{}
	cmd := exec.CommandContext(
		runCtx,
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
	cmd.Stdout = output
	cmd.Stderr = output

	if err := cmd.Start(); err != nil {
		log.Printf("ffmpeg failed to start for %s: %v", videoID, err)
		markVideoError(db, videoID)
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
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if _, err := os.Stat(firstSegment); err == nil {
					if err := storage.SetHLSPath(runCtx, db, videoID, hlsPublicPath); err != nil {
						log.Printf("failed to set hls_path for %s: %v", videoID, err)
					}
					cacheClient.Del(runCtx, cache.KeyVideoList, cache.KeyVideo(videoID))
					return
				}
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		log.Printf("ffmpeg failed for %s: %v - %s", videoID, err, strings.TrimSpace(output.String()))
		markVideoError(db, videoID)
		return
	}

	// Fresh context for terminal writes: if shutdown cancelled runCtx
	// mid-transcode we still want the status update to land.
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()
	if _, err := storage.MarkVideoReady(cleanupCtx, db, videoID, hlsPublicPath, 0); err != nil {
		log.Printf("failed to mark video %s ready: %v", videoID, err)
		markVideoError(db, videoID)
		return
	}
	cacheClient.Del(cleanupCtx, cache.KeyVideoList, cache.KeyVideo(videoID))
}

// markVideoError sets the video to "error" using a fresh short-lived context,
// so it works even when the calling goroutine's ctx was just cancelled.
func markVideoError(db *sql.DB, videoID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := storage.UpdateVideoStatus(ctx, db, videoID, "error"); err != nil {
		log.Printf("failed to mark video %s error: %v", videoID, err)
	}
}
