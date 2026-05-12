# CONTEXT CAPSULE - internal/storage

## Purpose
Encapsulates persistence and schema bootstrap for backend internals.
This package is intentionally under `internal/` to avoid external imports.

## Files
- `db.go`
  - Opens PostgreSQL via `github.com/jackc/pgx/v5/stdlib` (`sql.Open("pgx", dsn)`)
  - Pool: `MaxOpenConns(25)`, `MaxIdleConns(5)`, `ConnMaxLifetime(5min)`
  - Applies schema idempotente (`CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ADD COLUMN IF NOT EXISTS` para evolución)
  - Crea índices: `idx_videos_status`, `idx_videos_created_at`, `idx_streams_stream_key`, `idx_streams_status`, `idx_streams_created_at`
  - CHECK constraint en `streams.status` (`pending`/`live`/`ended`)
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
- Campos nullables convertidos con `sql.NullString` / `sql.NullTime`.
- Timestamps: `TIMESTAMPTZ NOT NULL DEFAULT NOW()` en Postgres.

## Cache Coupling
- Storage no conoce Redis. La capa API en `cmd/main.go` se encarga de invalidar las claves de cache (`videos:*`, `streams:*`) tras cada mutación.

## Tests
- Unit tests en `videos_test.go`, `streams_test.go` con `testhelper_test.go`. Cada test crea un schema único en `TEST_DATABASE_URL` (default = `DATABASE_URL`), corre contra él y lo dropea en cleanup; si Postgres no está accesible, los tests se skipean.
- E2E VOD en `../e2etest/vod_test.go` bajo build tag `e2e` (requiere backend + Postgres + ffmpeg corriendo).

## Known Gaps
- Migraciones inline en `db.go` (`CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ADD COLUMN IF NOT EXISTS`); no hay framework de migrations.
- Status de videos validados en capa API, no a nivel CHECK en DB (sí está en `streams`).
