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
Current schema allows:
- `pending`
- `live`
- `ended`

`video_id` is nullable to support live-first flow.

## Conventions
- All functions accept `context.Context`.
- Errors are wrapped with `%w`.
- Nullable DB fields are converted using `sql.NullString` helpers.

## Known Gaps
- No migrations framework yet (schema is bootstrap SQL string).
- No storage tests yet.
- Status values are validated in API layer, not DB-level CHECK for videos yet.
