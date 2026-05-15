# Streaming POC

POC de streaming con arquitectura `backend` (Go + Gin + PostgreSQL + FFmpeg) y `frontend` (Angular + Material + HLS.js).

## Estructura

- `backend/`: API, almacenamiento PostgreSQL, transcodificacion VOD a HLS, live streaming via RTMP y serving de playlists/segmentos.
- `frontend/streaming-frontend/`: UI admin/public para VOD y Live.
- `docker-compose.yml` + `mediamtx.yml`: MediaMTX como servidor RTMP para recibir streams de OBS.

## Requisitos

- Go 1.25+
- Node.js 22+
- npm 10+
- Docker (para MediaMTX y Redis vía `docker-compose.yml`)
- PostgreSQL 18 instalado y corriendo (fuera de Docker)
- `ffmpeg` disponible en PATH
- OBS (para emitir streams)
- ngrok instalado y autenticado (`ngrok config add-authtoken <token>`) — necesario solo para el despliegue con ngrok

## Setup de base de datos (primera vez)

Ejecutar en `psql` como superusuario:

```sql
CREATE USER streaming_user WITH PASSWORD 'streaming_pass';
CREATE DATABASE streaming OWNER streaming_user;
```

Las tablas se crean automáticamente al iniciar el backend.

## Levantar MediaMTX + Redis

```powershell
docker compose up -d
```

`docker-compose.yml` levanta dos servicios:

- **MediaMTX** en `:1935` (RTMP). Llama los hooks del backend al conectar/desconectar OBS.
- **Redis** en `:6379`. Cache opcional con TTLs cortos. Si Redis no está disponible el backend sigue funcionando en modo fail-soft (todo directo contra Postgres).

Postgres corre **fuera** del compose.

## Levantar backend

```powershell
cd backend
go run cmd/main.go
```

API por defecto: `http://localhost:8080`

### Variables de entorno

- `DATABASE_URL`: connection string de PostgreSQL. Default: `postgres://streaming_user:streaming_pass@localhost:5432/streaming?sslmode=disable`
- `REDIS_URL`: connection string de Redis. Default: `redis://localhost:6379`. Vacío o inalcanzable → backend corre sin cache (fail-soft).
- `MAX_UPLOAD_BYTES`: tope de upload en bytes. Default: 524288000 (500 MB). Excederlo devuelve 413.
- `CORS_ALLOWED_ORIGINS`: lista separada por comas de orígenes permitidos. Default: `http://localhost:4200,http://127.0.0.1:4200`.
- `CORS_ALLOW_NGROK=true`: permite `https://*.ngrok-free.app` y `https://*.ngrok.io`.
- `GIT_COMMIT`: opcional. Se expone como el campo `commit` en `GET /health`.
- `BACKEND_ROOT`: ruta del árbol del backend (donde viven `uploads/` y `media/`). Si no se setea, se resuelve via `runtime.Caller` (funciona con `go run`, no para binarios deployados fuera del repo).
- `TEST_DATABASE_URL`: usado por los tests del paquete `storage` y `handlers`. Default: igual a `DATABASE_URL`.

Ejemplo:

```powershell
$env:CORS_ALLOWED_ORIGINS="http://localhost:4200,https://tu-url.ngrok-free.app"
go run cmd/main.go
```

## Levantar frontend (desarrollo local)

```powershell
cd frontend/streaming-frontend
npm install
npm start
```

Frontend por defecto: `http://localhost:4200`

## Despliegue con ngrok

Para exponer la app a otra red sin servidor remoto. OBS debe correr en la misma máquina.

**1. Build del frontend**
```powershell
cd frontend/streaming-frontend
npm run build
```

El backend sirve automáticamente los archivos generados en `dist/streaming-frontend/browser/` — no hace falta levantar el dev server de Angular.

**2. Levantar MediaMTX y backend** (igual que en desarrollo local)

**3. Levantar ngrok**
```powershell
ngrok start --all
```

El config en `%USERPROFILE%\AppData\Local\ngrok\ngrok.yml` ya tiene el túnel HTTP al puerto 8080 configurado.

ngrok muestra la URL pública, por ejemplo:
```
Forwarding  https://xxxx.ngrok-free.app -> http://localhost:8080
```

Esa URL es la que se abre en el navegador. OBS sigue apuntando a `rtmp://localhost:1935/live`.

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
3. Crea registro en PostgreSQL con estado `pending`.
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
7. El backend promueve automáticamente el stream a un VOD `ready` apuntando a los mismos archivos HLS (sin retranscoding ni copia). El video aparece en la lista de VOD inmediatamente.

## Load testing (k6)

Requiere [k6](https://k6.io/docs/get-started/installation/) instalado.

**1. Configura `testing/.env`** (copia de `testing/.env.example`):
```env
BASE_URL=http://localhost:8080
VUS=10
DURATION=15m
VIDEO_ID=   # requerido para hls-vod
STREAM_ID=  # requerido para hls-live
```

**2. Corre el test:**
```powershell
.\testing\run.ps1 api-smoke   # prueba endpoints REST bajo carga
.\testing\run.ps1 hls-vod     # simula espectadores descargando un video
.\testing\run.ps1 hls-live    # simula espectadores en un stream en vivo
```

Los reportes HTML y JSON se generan en `testing/reports/`.

## Tests Go

**Unit tests del storage** (Postgres real contra schemas temporales aislados por test):

```powershell
cd backend
go test ./...
```

Cada test crea un schema único en la base configurada por `TEST_DATABASE_URL` (default igual a `DATABASE_URL`), corre contra él y lo dropea en cleanup. Si Postgres no está accesible los tests se skipean en vez de fallar.

**Smoke test e2e de VOD** (sube el fixture, espera transcoding, valida playlist y segmentos):

```powershell
cd backend
go test -tags=e2e ./internal/e2etest/...
```

Está bajo el build tag `e2e` para que no corra en `go test ./...`. Requiere backend + Postgres + ffmpeg en PATH. Usa el fixture en `testing/fixtures/sample.mp4` y deja un registro `e2e-vod-<timestamp>` por corrida (no hay cleanup automático).

## Notas

- Los segmentos HLS de streams terminados no se eliminan; los streams `ended` son reproducibles.
- Si el frontend no lista videos/streams, verifica que el backend esté corriendo y CORS permitido para tu origen.
