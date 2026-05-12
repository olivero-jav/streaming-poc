# CONTEXT CAPSULE - src/app

## Routing Snapshot
- `''` → redirect `/app/vod`
- `/app/vod` → `VodPageComponent`
- `/app/live` → `LivePageComponent`
- `/watch/vod/:id` → `WatchVodPageComponent`
- `/watch/live/:id` → `WatchLivePageComponent`
- fallback `**` → redirect `/app/vod`

## Core Components
- `NavComponent`: topbar con tabs VOD y Live
- `VodPageComponent`: admin VOD (player + grid + upload drawer)
- `WatchVodPageComponent`: vista pública VOD
- `LivePageComponent`: admin Live (lista streams, crea stream, muestra stream key, reproduce inline; polling 10s)
- `WatchLivePageComponent`: vista pública de un stream (polling 5s)
- `UploadDrawerComponent`: formulario de subida de video
- `VideoPlayerComponent`: wrapper reutilizable de hls.js

## Services
- `VideoApiService`: CRUD videos + `resolvePlaybackUrl`
- `StreamApiService`: CRUD streams + `resolvePlaybackUrl`

## Data Flow (VOD)
1. `VideoApiService.listVideos()` puebla `VodPage`.
2. `VodPage` selecciona el primer video `ready` o el actual si sigue presente.
3. `VideoApiService.resolvePlaybackUrl()` normaliza la URL del playlist.
4. `VideoPlayerComponent` reproduce via `hls.js`.

## Data Flow (Live)
1. `StreamApiService.listStreams()` puebla `LivePage` (polling cada 10s). `VodPage` también hace polling cada 10s.
2. `LivePage` auto-selecciona el primer stream `live` con `hls_path`.
3. `resolvePlaybackUrl()` convierte `hls_path` relativo a URL absoluta.
4. `VideoPlayerComponent` reproduce el HLS live/ended.

## Status Mapping (UI)
- `pending` -> `pendiente`
- `processing` -> `procesando`
- `ready` -> `listo`
- `error` -> `error`

## Recent Fixes To Remember
- Upload button enable logic uses method (`canSubmit()`) instead of signal `computed` over `form.valid`.
- File picker is triggered explicitly (`openFilePicker`) for reliable drawer upload interaction.
- Material chip status colors require deep selectors targeting MDC label classes.
