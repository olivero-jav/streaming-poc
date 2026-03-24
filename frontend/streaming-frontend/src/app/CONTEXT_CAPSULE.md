# CONTEXT CAPSULE - src/app

## Routing Snapshot
- `''` -> redirect `/app/vod`
- `/app/vod` -> `VodPageComponent`
- `/watch/vod/:id` -> `WatchVodPageComponent`
- fallback `**` -> redirect `/app/vod`

## Core Components
- `NavComponent`: topbar and VOD tab
- `VodPageComponent`: admin page (player + status + grid + upload drawer)
- `WatchVodPageComponent`: public watch page
- `UploadDrawerComponent`: upload form
- `VideoPlayerComponent`: reusable HLS player wrapper

## Data Flow
1. `VideoApiService.listVideos()` populates `VodPage`.
2. `VodPage` selects first `ready` video or current selected if still present.
3. `VideoApiService.resolvePlaybackUrl()` normalizes playlist URL.
4. `VideoPlayerComponent` plays normalized URL via `hls.js`.

## Status Mapping (UI)
- `pending` -> `pendiente`
- `processing` -> `procesando`
- `ready` -> `listo`
- `error` -> `error`

## Recent Fixes To Remember
- Upload button enable logic uses method (`canSubmit()`) instead of signal `computed` over `form.valid`.
- File picker is triggered explicitly (`openFilePicker`) for reliable drawer upload interaction.
- Material chip status colors require deep selectors targeting MDC label classes.
