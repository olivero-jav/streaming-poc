# Streaming POC

POC de streaming con arquitectura `backend` (Go + Gin + SQLite + FFmpeg) y `frontend` (Angular + Material + HLS.js).

## Estructura

- `backend/`: API, almacenamiento SQLite, transcodificacion VOD a HLS, live streaming via RTMP y serving de playlists/segmentos.
- `frontend/streaming-frontend/`: UI admin/public para VOD y Live.
- `docker-compose.yml` + `mediamtx.yml`: MediaMTX como servidor RTMP para recibir streams de OBS.

## Requisitos

- Go 1.25+
- Node.js 22+
- npm 10+
- Docker (para MediaMTX)
- `ffmpeg` disponible en PATH
- OBS (para emitir streams)

## Levantar MediaMTX

```powershell
docker compose up -d
```

Recibe RTMP en `:1935` y llama los hooks del backend al conectar/desconectar.

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
- Admin Live: `http://localhost:4200/app/live`
- Watch Live: `http://localhost:4200/watch/live/:id`

### Backend

- `GET /health`
- `POST /videos` (multipart: `title`, `description` opcional, `file`)
- `GET /videos`
- `GET /videos/:id`
- `PATCH /videos/:id/status` (uso interno en POC)
- `GET /hls/vod/:id/index.m3u8`
- `GET /hls/vod/:id/:segment`
- `POST /streams` (JSON: `title`)
- `GET /streams`
- `GET /streams/:id`
- `GET /hls/live/:id/index.m3u8`
- `GET /hls/live/:id/:segment`
- `POST /internal/hooks/publish?path=live/{streamKey}` (llamado por MediaMTX)
- `POST /internal/hooks/unpublish?path=live/{streamKey}` (llamado por MediaMTX)

## Flujo VOD actual

1. Subida de video desde frontend.
2. Backend guarda archivo en `backend/uploads/`.
3. Crea registro en SQLite (`backend/streaming.db`) con estado `pending`.
4. FFmpeg procesa en background:
   - `processing` -> `ready` o `error`
5. HLS generado en `backend/media/hls/<videoId>/`.
6. Frontend reproduce con HLS.js desde la URL del backend.

## Flujo Live

1. Crear stream desde `/app/live` — se genera un `stream_key`.
2. Configurar OBS: Service `Custom`, Server `rtmp://localhost/live`, Stream Key del paso anterior.
3. Al iniciar streaming en OBS, MediaMTX llama el hook `publish`.
4. Backend lanza FFmpeg que escribe HLS en `backend/media/live/<streamId>/`.
5. En ~6s el stream pasa a `live` y es reproducible desde la app.
6. Al detener OBS, MediaMTX llama `unpublish` → stream pasa a `ended`.

## Notas

- Los segmentos HLS de streams terminados no se eliminan; los streams `ended` son reproducibles.
- Si el frontend no lista videos/streams, verifica que el backend esté corriendo y CORS permitido para tu origen.
