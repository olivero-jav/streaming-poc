# CONTEXT CAPSULE - cmd

## Purpose
`cmd/main.go` wires the app:
- initializes SQLite
- registers HTTP routes
- starts Gin server on `:8080`

## Design Choices
- Single-file bootstrap for POC speed.
- Business/data logic delegated to `internal/storage`.
- Async FFmpeg processing started in goroutine after upload.
- Runtime paths are anchored to backend root via `runtime.Caller` (prevents accidental multiple DB files when running from different cwd).
- Local CORS middleware allows frontend on `:4200`.

## Important Routes
- Health: `GET /health`
- Videos:
  - `POST /videos` (multipart: `title`, optional `description`, required `file`)
  - `GET /videos`
  - `GET /videos/:id`
  - `PATCH /videos/:id/status`
- HLS VOD:
  - `GET /hls/vod/:id/index.m3u8`
  - `GET /hls/vod/:id/:segment`

## Security/Exposure Rules
- Do not expose whole media folder with generic static mount.
- Serve only expected HLS files through controlled handlers.
- Keep path validation in place (reject traversal patterns like `..`).
- CORS is configurable via env:
  - `CORS_ALLOWED_ORIGINS` (comma-separated exact origins)
  - `CORS_ALLOW_NGROK=true` to allow `https://*.ngrok-free.app` and `https://*.ngrok.io` for demos.

## Refactor Direction
As complexity increases, split `main.go` into:
- `api/videos.go`
- `api/hls.go`
- `worker/transcoder.go`
- route registration helpers
