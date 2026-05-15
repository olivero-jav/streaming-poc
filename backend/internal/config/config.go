// Package config centralizes the runtime configuration of the backend. All
// env-var parsing and defaulting lives here so that other packages receive a
// fully-resolved Config and do not call os.Getenv directly.
package config

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const DefaultMaxUploadBytes int64 = 500 << 20 // 500 MB

type Config struct {
	Addr               string
	DatabaseURL        string
	RedisURL           string
	MaxUploadBytes     int64
	GitCommit          string
	CORSAllowedOrigins []string
	CORSAllowNgrok     bool
	BackendRoot        string
}

func Load() (Config, error) {
	root, err := resolveBackendRoot()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Addr:               ":8080",
		DatabaseURL:        envOr("DATABASE_URL", "postgres://streaming_user:streaming_pass@localhost:5432/streaming?sslmode=disable"),
		RedisURL:           envOr("REDIS_URL", "redis://localhost:6379"),
		MaxUploadBytes:     DefaultMaxUploadBytes,
		GitCommit:          os.Getenv("GIT_COMMIT"),
		CORSAllowedOrigins: parseCORSOrigins(os.Getenv("CORS_ALLOWED_ORIGINS")),
		CORSAllowNgrok:     strings.EqualFold(strings.TrimSpace(os.Getenv("CORS_ALLOW_NGROK")), "true"),
		BackendRoot:        root,
	}

	if raw := os.Getenv("MAX_UPLOAD_BYTES"); raw != "" {
		parsed, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil || parsed <= 0 {
			log.Printf("invalid MAX_UPLOAD_BYTES=%q, falling back to %d", raw, DefaultMaxUploadBytes)
		} else {
			cfg.MaxUploadBytes = parsed
		}
	}

	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

var defaultCORSOrigins = []string{
	"http://localhost:4200",
	"http://127.0.0.1:4200",
}

func parseCORSOrigins(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]string(nil), defaultCORSOrigins...)
	}

	out := make([]string, 0)
	for _, entry := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(entry)
		if origin != "" {
			out = append(out, origin)
		}
	}
	return out
}

// resolveBackendRoot returns the on-disk path of the backend tree. Prefers the
// BACKEND_ROOT env var; falls back to walking up from this source file, which
// works while running from the repo (e.g. `go run ./cmd`) but not for a binary
// deployed away from its source tree — in that case BACKEND_ROOT must be set.
func resolveBackendRoot() (string, error) {
	if v := strings.TrimSpace(os.Getenv("BACKEND_ROOT")); v != "" {
		return filepath.Clean(v), nil
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime caller lookup failed")
	}
	// internal/config/config.go → backend/
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..")), nil
}
