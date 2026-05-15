package transcode

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"streaming-poc/backend/internal/cache"
	"streaming-poc/backend/internal/process"
	"streaming-poc/backend/internal/storage"
)

// StartLive captures an incoming RTMP stream into HLS files under
// {backendRoot}/media/live/{stream.ID}/, marks the stream live once the first
// segment lands, and on ffmpeg exit marks it ended and promotes its files to a
// VOD entry. Blocking by design; intended to run from a background goroutine.
// The ffmpeg invocation is bound to appCtx so shutdown kills it.
func StartLive(appCtx context.Context, db *sql.DB, cacheClient *cache.Client, backendRoot string, stream storage.Stream, registry *process.Registry) {
	hlsDir := filepath.Join(backendRoot, "media", "live", stream.ID)
	if err := os.MkdirAll(hlsDir, 0o755); err != nil {
		log.Printf("failed to create live hls dir for stream %s: %v", stream.ID, err)
		return
	}

	hlsPublicPath := "/hls/live/" + stream.ID + "/index.m3u8"
	playlistPath := filepath.Join(hlsDir, "index.m3u8")

	cmd := exec.CommandContext(
		appCtx,
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
			case <-appCtx.Done():
				return
			case <-deadline:
				log.Printf("timed out waiting for first live segment of stream %s", stream.ID)
				return
			case <-ticker.C:
				if _, err := os.Stat(filepath.Join(hlsDir, "index0.ts")); err == nil {
					if err := storage.MarkStreamLive(appCtx, db, stream.ID, hlsPublicPath); err != nil {
						log.Printf("failed to mark stream %s live: %v", stream.ID, err)
					}
					cacheClient.Del(appCtx, cache.KeyStreamList, cache.KeyStream(stream.ID))
					return
				}
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		log.Printf("ffmpeg exited for stream %s: %v", stream.ID, err)
	}

	registry.Kill(stream.StreamKey)

	// Fresh context so cleanup lands even if appCtx was just cancelled.
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()
	if err := storage.MarkStreamEnded(cleanupCtx, db, stream.ID); err != nil {
		log.Printf("failed to mark stream %s ended: %v", stream.ID, err)
	}
	cacheClient.Del(cleanupCtx, cache.KeyStreamList, cache.KeyStream(stream.ID))

	// Skip VOD promotion on shutdown: the new goroutine would outlive main().
	if appCtx.Err() == nil {
		go promoteStreamToVOD(db, cacheClient, backendRoot, stream)
	}
}

// promoteStreamToVOD creates a ready VOD entry from the HLS files produced
// during a live stream session. The files are served in place — no copy.
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
