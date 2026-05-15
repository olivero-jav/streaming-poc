# CONTEXT CAPSULE - Backend

## Purpose
Go backend para un POC de streaming:
- VOD upload + transcoding async a HLS
- Live streaming via RTMP (MediaMTX) → HLS, con auto-promoción a VOD al terminar
- Stream lifecycle (`pending`, `live`, `ended`)
- PostgreSQL persistence (pgx/v5)
- Redis cache (cache-aside, fail-soft)

## Package Layout
- `cmd/main.go` — bootstrap mínimo: carga config, abre DB y cache, arma `Deps`, llama a `server.Build` + `server.Run`.
- `internal/config/` — parsing y defaulting de env vars; resuelve `BackendRoot`.
- `internal/server/` — `Build` (middlewares + rutas) y `Run` (graceful shutdown con WaitGroup).
- `internal/api/` — middlewares y helpers transversales: CORS, security headers (CSP, HSTS, nosniff, etc.), `LimitUploadSize`, `DetectVideoMime`.
- `internal/handlers/` — HTTP handlers agrupados por dominio (videos, streams, hls, hooks, health, spa). Cada handler recibe `*Deps` y devuelve `gin.HandlerFunc`.
- `internal/storage/` — Postgres CRUD para `videos` y `streams`, schema bootstrap idempotente, unit tests con schemas temporales + e2e bajo build tag.
- `internal/cache/` — wrapper Redis fail-soft (cache-aside).
- `internal/transcode/` — orquestación de ffmpeg para VOD y Live; semáforo de concurrencia, `lockedBuffer` para stdout/stderr serializados.
- `internal/process/` — registro de procesos ffmpeg keyed por stream key, para `Kill` desde el hook unpublish.
- `internal/e2etest/` — smoke e2e VOD bajo build tag `e2e`.

## HTTP Surface
- Health: `GET /health` → `{status, redis_up, commit}` (`commit` desde env `GIT_COMMIT`).
- VOD:
  - `POST /videos` (multipart: `title`, optional `description`, `file`). Devuelve `202`. Valida MIME real (`mp4/webm/quicktime`, mkv rechazado) leyendo magic bytes; `MaxBytesReader` con cap `MAX_UPLOAD_BYTES` (default 500MB) → 413 si excede.
  - `GET /videos`
  - `GET /videos/:id`
  - `PATCH /videos/:id/status` (uso interno; sin auth)
  - `GET /hls/vod/:id/index.m3u8`
  - `GET /hls/vod/:id/:segment`
- Live:
  - `POST /streams` (JSON: `title`) — genera `stream_key`
  - `GET /streams`
  - `GET /streams/:id`
  - `GET /hls/live/:id/index.m3u8`
  - `GET /hls/live/:id/:segment`
- MediaMTX hooks (sin auth, por convención de localhost):
  - `POST /internal/hooks/publish?path=live/{streamKey}` → arranca ffmpeg para el stream
  - `POST /internal/hooks/unpublish?path=live/{streamKey}` → mata ffmpeg via registry
- SPA fallback: si existe `frontend/streaming-frontend/dist/streaming-frontend/browser/`, se sirve como SPA con `NoRoute` (fallback a `index.html`). Si no existe, no se registra (dev local en `:4200`).

## Middlewares globales
- `gin.Default` (logger + recovery)
- `api.CORS` (allow-list por env; opcionalmente `*.ngrok-*` con `CORS_ALLOW_NGROK=true`)
- `gzip` (excluye `.ts/.m4s/.mp4/.aac` para no recomprimir media)
- `api.SecurityHeaders` (nosniff, Referrer-Policy, X-Frame-Options, HSTS sobre HTTPS). CSP solo en respuestas HTML (lo agrega el handler SPA).

## Key Runtime Flow (VOD)
1. Cliente sube archivo multipart → `internal/handlers/videos.go::UploadVideo`.
2. Backend guarda en `backend/uploads/`.
3. Fila en Postgres con `status=pending`.
4. `Deps.BgRun` lanza `transcode.ProcessVideo` (semáforo de 4 concurrentes). FFmpeg transcodifica a HLS en `backend/media/hls/{videoID}/`.
5. Tan pronto aparece `index0.ts`, se setea `hls_path` y se invalida cache → el cliente puede empezar a reproducir antes de terminar el transcoding.
6. Al terminar: `MarkVideoReady` (status `ready`). Si falla, `markVideoError` con fresh context.
7. Cada transición invalida `videos:list` + `videos:{id}` en Redis.

## Key Runtime Flow (Live)
1. OBS conecta por RTMP a MediaMTX (`rtmp://localhost:1935/live/{streamKey}`).
2. MediaMTX llama `POST /internal/hooks/publish?path=live/{streamKey}`.
3. `Deps.BgRun` lanza `transcode.StartLive`: ffmpeg lee el RTMP (copy video, AAC audio) y escribe HLS en `backend/media/live/{streamID}/`.
4. Cuando aparece el primer segmento, `MarkStreamLive` setea `status=live` + `hls_path` + `started_at`.
5. OBS desconecta → MediaMTX llama `unpublish` → `registry.Kill` envía SIGINT a ffmpeg (kill -9 si no muere en 5s).
6. `MarkStreamEnded` + invalidación de cache.
7. Si no estamos en shutdown, `promoteStreamToVOD` crea un VOD `ready` apuntando a los mismos archivos HLS live (sin retranscoding ni copia). `finalizeHLSPlaylist` añade `#EXT-X-ENDLIST` si ffmpeg fue killado sin cerrarse limpiamente.

## Data Model Notes
- `videos`: estado de procesamiento + metadata HLS. Indexado por `status` y `created_at DESC`. Status validado en capa de handlers (no CHECK en DB).
- `streams`: title, stream_key (unique), status, hls_path, started_at, ended_at. Sin FK a videos. Indexado por `stream_key`, `status` y `created_at DESC`. CHECK constraint en status (`pending`/`live`/`ended`).

## Cache Layer
- Redis cache-aside con TTLs cortos (30s listas, 60s ítems). Claves: `videos:list`, `videos:{id}`, `streams:list`, `streams:{id}`. TTLs viven en `internal/handlers/{videos,streams}.go`.
- Fail-soft: si `REDIS_URL` falla o el ping no responde, el cliente queda nil y todas las operaciones son no-op. El backend sigue sirviendo desde Postgres.
- Invalidación explícita en cada mutación.
- RAM-only (sin volumen en docker-compose): Postgres es la fuente de verdad.

## Graceful Shutdown
- `signal.NotifyContext` cancela `appCtx` en SIGINT/SIGTERM.
- Todos los ffmpeg están bound a `appCtx` via `exec.CommandContext`, así que la cancelación los mata.
- `server.Run` hace `srv.Shutdown` con 10s de budget, después drena `BgWG` con 15s adicionales.
- "Fresh context" en finalizaciones (`transcode/vod.go`, `live.go`) para que los UPDATE de cierre lleguen aun si `runCtx` ya se canceló.

## Operational Notes
- `ffmpeg` debe estar en PATH.
- MediaMTX debe estar corriendo y configurado para llamar los hooks (ver `mediamtx.yml`).
- `BackendRoot` se resuelve desde la env `BACKEND_ROOT`; fallback a `runtime.Caller` desde `internal/config/config.go` (funciona corriendo con `go run`, no para binarios deployeados fuera del repo).
- Al iniciar, `ResetStaleStreams` pasa streams `live` a `ended` (limpieza de crashes previos) y se invalida `streams:list` en Redis.
- Env vars: `DATABASE_URL`, `REDIS_URL`, `MAX_UPLOAD_BYTES`, `CORS_ALLOWED_ORIGINS`, `CORS_ALLOW_NGROK`, `GIT_COMMIT`, `BACKEND_ROOT`, `TEST_DATABASE_URL`. Defaults en `internal/config/config.go`.

## Near-Term TODOs
- Hooks `/internal/hooks/*` sin auth: cualquiera con acceso al puerto 8080 puede dispararlos si conoce un `stream_key`. Cerrar con shared-secret header o filtro por loopback.
- `promoteStreamToVOD` se spawnea como goroutine fuera del `BgWG` (`transcode/live.go`). Ventana de race en shutdown.
- `process.Registry.Kill` lanza una goroutine "force-kill en 5s" untracked.
- Migraciones inline (`CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ADD COLUMN IF NOT EXISTS`). Considerar golang-migrate/goose.
- `ffprobe` para persistir `duration_seconds` real en videos.
- Cleanup de segmentos live de streams `ended` antiguos.
- Sin rate limiting ni structured logging.
- `internal/processor/` quedó vacío tras el refactor; eliminarlo.
