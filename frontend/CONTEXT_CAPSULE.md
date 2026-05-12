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
- `API_BASE_URL` se resuelve en runtime con `resolveApiBaseUrl()` (en `video-api.service.ts`): `http://localhost:8080` cuando `hostname` es `localhost`/`127.0.0.1`, y `window.location.origin` en cualquier otro caso (ngrok/prod, donde el mismo backend Go sirve el build de Angular).
- Backend retorna `hls_path` como ruta relativa; frontend la convierte a URL absoluta antes del player.
- `StreamApiService.resolvePlaybackUrl()` maneja tanto rutas relativas como absolutas.

## UX Rules Currently Enforced
- UI en español.
- Tab Live habilitado y funcional.
- Solo videos con `status=ready` son seleccionables/clicables en VOD.
- Upload FAB se oculta mientras el drawer está abierto.
- VOD page y Live page hacen polling cada 10s para refrescar la lista.
- Watch Live page hace polling cada 5s para refrescar estado del stream.
- Streams `ended` son reproducibles (segmentos HLS quedan en disco).

## Known Risks / Follow-ups
- Sin banner global de "backend offline".
- Rutas públicas sin auth por diseño en POC.
