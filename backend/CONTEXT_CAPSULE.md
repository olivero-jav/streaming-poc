# CONTEXT CAPSULE - Backend

## Purpose
Go backend for a streaming POC with:
- VOD upload and async processing to HLS
- Live streaming via RTMP (MediaMTX) → HLS
- Stream lifecycle (`pending`, `live`, `ended`)
- SQLite persistence

## Current Scope
- HTTP server in `cmd/main.go` (Gin)
- Storage layer in `internal/storage/`
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
3. Fila en SQLite con `status=pending`.
4. FFmpeg async transcodifica a HLS → `backend/media/hls/{videoID}/`.
5. Estado avanza: `processing` → `ready` (o `error`).

## Key Runtime Flow (Live)
1. OBS/streamer conecta por RTMP a MediaMTX (`rtmp://localhost:1935/live/{streamKey}`).
2. MediaMTX llama `POST /internal/hooks/publish?path=live/{streamKey}`.
3. Backend lanza ffmpeg que lee el RTMP y escribe HLS en `backend/media/live/{streamID}/`.
4. Cuando aparece el primer segmento, el stream pasa a `status=live`.
5. Al desconectarse OBS, MediaMTX llama `POST /internal/hooks/unpublish`.
6. Registry mata ffmpeg → stream pasa a `status=ended`.
7. Los segmentos HLS quedan en disco (no se borran); el stream es reproducible post-ended.

## Data Model Notes
- `videos`: estado de procesamiento + metadata HLS.
- `streams`: title, stream_key (RTMP), status, hls_path, started_at, ended_at. No tiene FK a videos.

## Operational Notes
- `ffmpeg` debe estar en PATH.
- MediaMTX debe estar corriendo y configurado para llamar los hooks.
- Runtime paths resueltos desde `cmd/main.go` via `runtime.Caller`.
- Al iniciar, `ResetStaleStreams` pasa streams `live` a `ended` (limpieza de crashes previos).
- `PATCH /videos/:id/status` sin auth; aceptable para POC local.

## Near-Term TODOs
- Usar `ffprobe` para persistir `duration_seconds` real en videos.
- Agregar chequeo de dependencias (ffmpeg/ffprobe) al startup.
- Los segmentos live no se limpian al terminar el stream; evaluar cleanup o conversión a VOD.
- Agregar tests para handlers y transiciones de storage.
