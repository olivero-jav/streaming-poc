# CONTEXT CAPSULE - internal/storage

## Purpose
Encapsulates persistence and schema bootstrap for backend internals.
This package is intentionally under `internal/` to avoid external imports.

## Files
- `db.go`
  - Opens SQLite (`modernc.org/sqlite`)
  - Applies schema (`videos`, `streams`)
  - Connection limits tuned for SQLite (`MaxOpenConns(1)`)
- `videos.go`
  - CRUD-ish helpers for VOD lifecycle:
    - `CreateVideo`
    - `CreateVideoFromStream` — crea VOD con `status=ready` directamente desde un stream terminado; `hls_path` apunta a `/hls/live/{streamID}/index.m3u8` (sin transcoding ni copia de archivos)
    - `ListVideos`
    - `GetVideoByID`
    - `UpdateVideoStatus`
    - `MarkVideoReady`

## Video Status Contract
Accepted statuses used by API layer:
- `pending`
- `processing`
- `ready`
- `error`

`hls_path` is stored as backend-relative route (example: `/hls/vod/<videoID>/index.m3u8`).
Frontend resolves this into an absolute API URL before playback.

## Stream Status Contract
Statuses: `pending` → `live` → `ended`

Campos en `streams`: `id`, `title`, `stream_key` (unique), `status`, `hls_path`, `started_at`, `ended_at`, `created_at`, `updated_at`. Sin FK a videos.

Funciones en `streams.go`:
- `CreateStream`, `ListStreams`, `GetStreamByID`, `GetStreamByKey`
- `MarkStreamLive` (setea `hls_path` + `started_at`)
- `MarkStreamEnded` (setea `ended_at`)
- `ResetStaleStreams` (limpieza al startup)

## Conventions
- Todas las funciones aceptan `context.Context`.
- Errores wrapeados con `%w`.
- Campos nullables convertidos con `sql.NullString`.

## Known Gaps
- Migraciones inline en `db.go` (ALTER TABLE ADD COLUMN); no hay framework de migrations.
- Sin tests de storage todavía.
- Status de videos validados en capa API, no a nivel CHECK en DB.
