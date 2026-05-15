package handlers

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"streaming-poc/backend/internal/api"
)

// SPAFallback returns a gin.HandlerFunc suitable for r.NoRoute that serves the
// Angular build under distDir. Static files inside distDir are served with
// long Cache-Control; anything else falls back to index.html so the Angular
// router can handle the route client-side.
//
// Returns nil if distDir does not exist on disk, so the caller can decide
// whether to register the NoRoute handler at all.
func SPAFallback(distDir string) gin.HandlerFunc {
	if _, err := os.Stat(distDir); err != nil {
		return nil
	}
	log.Printf("Serving Angular frontend from %s", distDir)

	cleanDist := filepath.Clean(distDir)

	return func(c *gin.Context) {
		urlPath := c.Request.URL.Path
		filePath := filepath.Clean(filepath.Join(distDir, filepath.FromSlash(urlPath)))
		if filePath != cleanDist && !strings.HasPrefix(filePath, cleanDist+string(filepath.Separator)) {
			c.Status(http.StatusForbidden)
			return
		}
		if info, statErr := os.Stat(filePath); statErr == nil && !info.IsDir() {
			if strings.ToLower(filepath.Ext(filePath)) == ".html" {
				c.Header("Cache-Control", "no-cache")
				c.Header("Content-Security-Policy", api.ContentSecurityPolicy())
			} else {
				c.Header("Cache-Control", "max-age=3600, public")
			}
			c.File(filePath)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Header("Content-Security-Policy", api.ContentSecurityPolicy())
		c.File(filepath.Join(distDir, "index.html"))
	}
}
