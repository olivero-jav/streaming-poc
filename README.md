# Streaming POC

POC de streaming con arquitectura `backend` (Go + Gin + SQLite + FFmpeg) y `frontend` (Angular + Material + HLS.js).

## Estructura

- `backend/`: API, almacenamiento SQLite, transcodificacion VOD a HLS y serving de playlists/segmentos.
- `frontend/streaming-frontend/`: UI admin/public para VOD.

## Requisitos

- Go 1.25+
- Node.js 22+
- npm 10+
- `ffmpeg` disponible en PATH

## Levantar backend

```powershell
cd backend
go run cmd/main.go
```

API por defecto: `http://localhost:8080`

### Variables de entorno CORS

- `CORS_ALLOWED_ORIGINS`: lista separada por comas de orígenes permitidos.
- `CORS_ALLOW_NGROK=true`: permite `https://*.ngrok-free.app` y `https://*.ngrok.io`.

Ejemplo:

```powershell
$env:CORS_ALLOWED_ORIGINS="http://localhost:4200,https://tu-url.ngrok-free.app"
go run cmd/main.go
```

## Levantar frontend

```powershell
cd frontend/streaming-frontend
npm install
npm start
```

Frontend por defecto: `http://localhost:4200`

## Rutas actuales

### Frontend

- Admin VOD: `http://localhost:4200/app/vod`
- Watch VOD: `http://localhost:4200/watch/vod/:id`

### Backend

- `GET /health`
- `POST /videos` (multipart: `title`, `description` opcional, `file`)
- `GET /videos`
- `GET /videos/:id`
- `PATCH /videos/:id/status` (uso interno en POC)
- `GET /hls/vod/:id/index.m3u8`
- `GET /hls/vod/:id/:segment`

## Flujo VOD actual

1. Subida de video desde frontend.
2. Backend guarda archivo en `backend/uploads/`.
3. Crea registro en SQLite (`backend/streaming.db`) con estado `pending`.
4. FFmpeg procesa en background:
   - `processing` -> `ready` o `error`
5. HLS generado en `backend/media/hls/<videoId>/`.
6. Frontend reproduce con HLS.js desde la URL del backend.

## Notas

- Este repo esta en iteracion VOD-first; live esta planificado para la siguiente fase.
- Si el frontend no lista videos, verifica primero que backend este corriendo y CORS permitido para tu origen.
