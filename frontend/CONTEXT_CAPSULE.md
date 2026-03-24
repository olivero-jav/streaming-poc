# CONTEXT CAPSULE - Frontend

## Purpose
Angular frontend for a streaming POC that targets VOD + Live.
Current implementation in this iteration is VOD-first.

## Current Scope
Implemented now:
- Admin route: `/app/vod`
  - list videos
  - select `ready` videos for playback
  - upload new videos with drawer form
- Public route: `/watch/vod/:id`
  - playback-only view

Planned next (not implemented yet):
- Admin route: `/app/live`
- Public route: `/watch/live/:streamId`
- Live control and playback UX

## Runtime Dependencies
- Frontend dev server: `http://localhost:4200`
- Backend API: `http://localhost:8080`
- HLS playback via `hls.js`

## Important Integration Rules
- API service uses fixed `API_BASE_URL` pointing to backend.
- Backend returns `hls_path` as relative route.
- Frontend must convert relative `hls_path` to absolute API URL before passing to player.

## UX Rules Currently Enforced
- UI language is Spanish.
- Live tab is hidden until live iteration is implemented.
- Only videos with status `ready` are selectable/clickable for playback.
- Upload FAB hides while drawer is open.

## Known Risks / Follow-ups
- `API_BASE_URL` is hardcoded (move to environments for staging/ngrok).
- No global "backend offline" banner yet.
- Public watch routes have no auth by design for POC.
