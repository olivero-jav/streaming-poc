# CONTEXT CAPSULE - cmd

## Purpose
`cmd/main.go` wires the app:
- initializes SQLite (+ reset de streams stale)
- crea el process registry
- registra todas las rutas HTTP
- arranca Gin en `:8080`

## Design Choices
- Single-file bootstrap para velocidad de POC.
- Lógica de negocio/datos delegada a `internal/storage` e `internal/process`.
- FFmpeg VOD arranca en goroutine tras el upload.
- FFmpeg Live arranca en goroutine cuando MediaMTX llama el hook publish.
- Runtime paths anclados al backend root via `runtime.Caller`.
- CORS middleware local permite frontend en `:4200`.

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
- Streams:
  - `POST /streams` (JSON: `title`)
  - `GET /streams`
  - `GET /streams/:id`
- HLS Live:
  - `GET /hls/live/:id/index.m3u8`
  - `GET /hls/live/:id/:segment`
- MediaMTX hooks (solo desde localhost):
  - `POST /internal/hooks/publish?path=live/{streamKey}`
  - `POST /internal/hooks/unpublish?path=live/{streamKey}`

## Security/Exposure Rules
- No exponer carpeta media con static mount genérico.
- Servir solo archivos HLS esperados via handlers controlados.
- Validación de path en todos los handlers (rechaza `..`).
- CORS configurable via env:
  - `CORS_ALLOWED_ORIGINS` (orígenes exactos separados por coma)
  - `CORS_ALLOW_NGROK=true` para permitir `https://*.ngrok-free.app` y `https://*.ngrok.io`

## Refactor Direction
Al crecer la complejidad, separar `main.go` en:
- `api/videos.go`
- `api/streams.go`
- `api/hls.go`
- `worker/transcoder.go`
- helpers de registro de rutas
