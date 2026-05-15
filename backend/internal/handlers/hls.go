package handlers

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// ServeHLSPlaylist returns a handler for GET /hls/{baseSubdir}/:id/index.m3u8.
// errorLabel is the human-readable name used in 400 responses (e.g. "video id"
// or "stream id").
func ServeHLSPlaylist(d *Deps, baseSubdir, errorLabel string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" || strings.Contains(id, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + errorLabel})
			return
		}
		serveHLSFile(c, filepath.Join(d.Cfg.BackendRoot, "media", baseSubdir, id), "index.m3u8")
	}
}

// ServeHLSSegment returns a handler for GET /hls/{baseSubdir}/:id/:segment.
func ServeHLSSegment(d *Deps, baseSubdir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		segment := strings.TrimSpace(c.Param("segment"))
		if id == "" || segment == "" || strings.Contains(id, "..") || strings.Contains(segment, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
			return
		}
		serveHLSFile(c, filepath.Join(d.Cfg.BackendRoot, "media", baseSubdir, id), segment)
	}
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
