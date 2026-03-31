# CONTEXT CAPSULE - Frontend

## Purpose
Angular frontend para un POC de streaming VOD + Live.

## Current Scope
Implementado:
- Admin route: `/app/vod` — lista videos, selecciona `ready` para reproducir, sube videos con drawer
- Public route: `/watch/vod/:id` — vista de reproducción
- Admin route: `/app/live` — lista streams, crea streams, muestra stream key para OBS, reproduce live/ended inline
- Public route: `/watch/live/:id` — vista de reproducción de un stream (polling cada 3s)

## Runtime Dependencies
- Frontend dev server: `http://localhost:4200`
- Backend API: `http://localhost:8080`
- HLS playback via `hls.js`

## Important Integration Rules
- `API_BASE_URL` hardcodeado (mover a environments para staging/ngrok).
- Backend retorna `hls_path` como ruta relativa; frontend la convierte a URL absoluta antes del player.
- `StreamApiService.resolvePlaybackUrl()` maneja tanto rutas relativas como absolutas.

## UX Rules Currently Enforced
- UI en español.
- Tab Live habilitado y funcional.
- Solo videos con `status=ready` son seleccionables/clicables en VOD.
- Upload FAB se oculta mientras el drawer está abierto.
- Live page hace polling cada 5s para actualizar la lista de streams.
- Watch Live page hace polling cada 3s para refrescar estado del stream.
- Streams `ended` son reproducibles (segmentos HLS quedan en disco).

## Known Risks / Follow-ups
- `API_BASE_URL` hardcodeado (mover a environments).
- Sin banner global de "backend offline".
- Rutas públicas sin auth por diseño en POC.
