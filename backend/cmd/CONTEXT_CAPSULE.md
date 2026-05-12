# CONTEXT CAPSULE - cmd

## Purpose
`cmd/main.go` wires the app:
- initializes Postgres (`DATABASE_URL`) y Redis cache (`REDIS_URL`, fail-soft)
- ejecuta `ResetStaleStreams` y limpia `streams:list` del cache
- crea el process registry
- registra todas las rutas HTTP (con gzip middleware)
- arranca Gin en `:8080`

## Design Choices
- Single-file bootstrap para velocidad de POC.
- Lógica de negocio/datos delegada a `internal/storage`, `internal/cache` e `internal/process`.
- Cache-aside en lecturas (`GET /videos`, `GET /videos/:id`, `GET /streams`, `GET /streams/:id`) con TTLs 30s/60s.
- Invalidación explícita del cache tras cada mutación (create, status update, lifecycle transitions live/ended, promoción a VOD).
- FFmpeg VOD arranca en goroutine tras el upload (con `transcodeSem` que limita a 4 transcodings concurrentes).
- Upload validado a nivel de bytes: `MaxBytesReader` con cap `MAX_UPLOAD_BYTES` (default 500MB) → 413, y `detectVideoMime` lee los primeros 512 bytes y los pasa por `http.DetectContentType` (acepta `mp4/webm/quicktime`; mkv excluido porque stdlib lo detecta mal).
- FFmpeg Live arranca en goroutine cuando MediaMTX llama el hook publish.
- Al terminar un stream, `promoteStreamToVOD` crea automáticamente un VOD apuntando a los archivos HLS live existentes (sin retranscoding ni copia). `finalizeHLSPlaylist` añade `#EXT-X-ENDLIST` si FFmpeg fue killado sin cerrarse limpiamente.
- Runtime paths anclados al backend root via `runtime.Caller`.
- CORS middleware local permite frontend en `:4200`.
- Cache-Control headers diferenciados: HLS playlists `no-cache`, segmentos `max-age=3600`, assets estáticos del frontend `max-age=31536000, immutable`.

## Important Routes
- Health: `GET /health` → `{status, redis_up, commit}` (`commit` desde env `GIT_COMMIT`)
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
