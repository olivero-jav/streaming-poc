package api

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AllowedVideoMimes are the MIME types accepted by the upload endpoint. Matches
// what http.DetectContentType returns for typical video files; MKV is excluded
// because Go's stdlib detection often falls back to application/octet-stream.
var AllowedVideoMimes = map[string]bool{
	"video/mp4":       true,
	"video/webm":      true,
	"video/quicktime": true,
}

// LimitUploadSize wraps the request body with http.MaxBytesReader so that any
// upload larger than limit bytes is rejected before it can be parsed. Trips
// with *http.MaxBytesError, which the handler translates to a 413 response.
func LimitUploadSize(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

// DetectVideoMime reads the first 512 bytes of the uploaded file and runs
// http.DetectContentType on them. Stronger than checking the filename
// extension or the multipart Content-Type header (both client-spoofable);
// magic bytes live in the actual content.
func DetectVideoMime(fileHeader *multipart.FileHeader) (string, error) {
	f, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("open uploaded file: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", fmt.Errorf("read header bytes: %w", err)
	}
	return http.DetectContentType(buf[:n]), nil
}
