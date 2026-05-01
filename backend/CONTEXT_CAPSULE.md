# CONTEXT CAPSULE - Backend

## Purpose
Go backend for a streaming POC with:
- VOD upload and async processing to HLS
- Live streaming via RTMP (MediaMTX) → HLS
- Stream lifecycle (`pending`, `live`, `ended`)
- PostgreSQL persistence (pgx/v5)
- Redis cache (cache-aside, fail-soft)

## Current Scope
- HTTP server in `cmd/main.go` (Gin)
- Storage layer in `internal/storage/` (Postgres)
- Cache layer in `internal/cache/` (Redis, fail-soft)
- Process registry in `internal/process/`
- VOD endpoints:
  - `POST /videos` (multipart upload, returns `202`)
  - `GET /videos`
  - `GET /videos/:id`
  - `PATCH /videos/:id/status` (internal-use; currently unauthenticated)
- HLS VOD serving:
  - `GET /hls/vod/:id/index.m3u8`
  - `GET /hls/vod/:id/:segment`
- Stream endpoints:
  - `POST /streams` (creates stream with a generated stream key)
  - `GET /streams`
  - `GET /streams/:id`
- HLS Live serving:
  - `GET /hls/live/:id/index.m3u8`
  - `GET /hls/live/:id/:segment`
- MediaMTX hooks (localhost only):
  - `POST /internal/hooks/publish` → arranca ffmpeg para el stream
  - `POST /internal/hooks/unpublish` → mata ffmpeg via registry
- CORS configurable via env; por defecto permite `http://localhost:4200` y `http://127.0.0.1:4200`
- Sirve el build estático de Angular desde `frontend/streaming-frontend/dist/streaming-frontend/browser/` si existe; fallback a `index.html` para rutas SPA. Si no existe el dist, el comportamiento no cambia (dev local con Angular en 4200).

## Key Runtime Flow (VOD)
1. Cliente sube archivo multipart.
2. Backend guarda en `backend/uploads/`.
3. Fila en Postgres con `status=pending`.
4. FFmpeg async transcodifica a HLS → `backend/media/hls/{videoID}/`.
5. Estado avanza: `processing` → `ready` (o `error`).
6. Cada transición invalida `videos:list` + `videos:{id}` en Redis.

## Key Runtime Flow (Live)
1. OBS/streamer conecta por RTMP a MediaMTX (`rtmp://localhost:1935/live/{streamKey}`).
2. MediaMTX llama `POST /internal/hooks/publish?path=live/{streamKey}`.
3. Backend lanza ffmpeg que lee el RTMP y escribe HLS en `backend/media/live/{streamID}/`.
4. Cuando aparece el primer segmento, el stream pasa a `status=live`.
5. Al desconectarse OBS, MediaMTX llama `POST /internal/hooks/unpublish`.
6. Registry mata ffmpeg → stream pasa a `status=ended`.
7. Los segmentos HLS quedan en disco (no se borran); el stream es reproducible post-ended.

## Data Model Notes
- `videos`: estado de procesamiento + metadata HLS. Indexado por `status` y `created_at DESC`.
- `streams`: title, stream_key (RTMP), status, hls_path, started_at, ended_at. No tiene FK a videos. Indexado por `stream_key`, `status` y `created_at DESC`. CHECK constraint en status.

## Cache Layer
- Redis cache-aside con TTLs cortos (30s listas, 60s ítems). Claves: `videos:list`, `videos:{id}`, `streams:list`, `streams:{id}`.
- Fail-soft: si `REDIS_URL` falla o el ping no responde, el cliente queda nil y todas las operaciones son no-op. El backend sigue sirviendo desde Postgres sin Redis.
- Invalidación explícita en cada mutación (create/update/status changes/lifecycle transitions).
- Sin persistencia en Redis (RAM-only, sin volumen): Postgres es la fuente de verdad.

## Operational Notes
- `ffmpeg` debe estar en PATH.
- MediaMTX debe estar corriendo y configurado para llamar los hooks.
- Runtime paths resueltos desde `cmd/main.go` via `runtime.Caller`.
- Al iniciar, `ResetStaleStreams` pasa streams `live` a `ended` (limpieza de crashes previos) y se invalida `streams:list` en Redis.
- `PATCH /videos/:id/status` sin auth; aceptable para POC local.
- Env vars: `DATABASE_URL` (default `postgres://streaming_user:streaming_pass@localhost:5432/streaming?sslmode=disable`), `REDIS_URL` (default `redis://localhost:6379`).

## Near-Term TODOs
- Usar `ffprobe` para persistir `duration_seconds` real en videos.
- Agregar chequeo de dependencias (ffmpeg/ffprobe) al startup.
- Los segmentos live no se limpian al terminar el stream; evaluar cleanup o conversión a VOD.
- Agregar tests para handlers y transiciones de storage.
