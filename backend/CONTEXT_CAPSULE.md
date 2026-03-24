# CONTEXT CAPSULE - Backend

## Purpose
Go backend for a streaming POC with:
- VOD upload and async processing to HLS
- Stream lifecycle groundwork (`pending`, `live`, `ended`)
- SQLite persistence

## Current Scope
- HTTP server in `cmd/main.go` (Gin)
- Storage layer in `internal/storage/`
- VOD endpoints:
  - `POST /videos` (multipart upload, returns `202`)
  - `GET /videos`
  - `GET /videos/:id`
  - `PATCH /videos/:id/status` (internal-use for now; currently unauthenticated)
- HLS VOD serving endpoints (controlled, no broad static exposure):
  - `GET /hls/vod/:id/index.m3u8`
  - `GET /hls/vod/:id/:segment`
- CORS enabled for local frontend:
  - `http://localhost:4200`
  - `http://127.0.0.1:4200`

## Key Runtime Flow (VOD)
1. Client uploads file using multipart.
2. Backend saves source file under `backend/uploads/`.
3. Video row is created in SQLite (`status=pending`).
4. Background FFmpeg job sets:
   - `processing` while transcoding
   - `ready` on success (`hls_path` set)
   - `error` on failure
5. HLS playlist/segments are stored under `backend/media/hls/{videoID}/`.

## Data Model Notes
- `videos` has processing state and HLS metadata.
- `streams` exists for live pipeline evolution and allows nullable `video_id`.

## Operational Notes
- `ffmpeg` must be available in PATH for async processing.
- Runtime paths are deterministic and resolved from source location (`cmd/main.go`):
  - DB file: `backend/streaming.db`
  - uploads: `backend/uploads/`
  - HLS files: `backend/media/hls/...`
- Generated assets (`uploads`, `media`, DB) are ignored by `.gitignore`.
- `PATCH /videos/:id/status` is exposed without auth in this POC; acceptable for local/demo use, but should be restricted before public use.

## Near-Term TODOs
- Priority: use `ffprobe` to persist real `duration_seconds` (currently videos end as `0`).
- Add ffmpeg/ffprobe dependency checks at startup.
- Add dedicated handlers/modules (`api/`, `hls.go`) as code grows.
- Add tests for handlers and storage transitions.
