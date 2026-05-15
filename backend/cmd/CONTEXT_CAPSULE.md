# CONTEXT CAPSULE - cmd

## Purpose
`cmd/main.go` es el entrypoint del proceso. Hace solo bootstrap:
1. `config.Load()` — parsea env vars (DATABASE_URL, REDIS_URL, MAX_UPLOAD_BYTES, CORS_*, GIT_COMMIT, BACKEND_ROOT).
2. `signal.NotifyContext(SIGINT, SIGTERM)` — crea `appCtx` que cancela todo ffmpeg/transcode/live spawneado bajo él.
3. `storage.InitPostgres` — abre la conexión y aplica schema idempotente.
4. `cache.New` — conecta a Redis (fail-soft: si falla, sigue sin cache).
5. `storage.ResetStaleStreams` — pasa streams `live` colgados a `ended` (cleanup post-crash) e invalida `streams:list` en cache.
6. Arma `handlers.Deps` con DB, Cache, `process.Registry`, config, appCtx, semáforo de transcoding (4) y un `sync.WaitGroup` para background goroutines.
7. `server.Build(deps)` arma el `*gin.Engine` con middlewares y rutas.
8. `server.Run` arranca y bloquea hasta SIGINT/SIGTERM; al cerrar, `srv.Shutdown(10s)` + drena `BgWG(15s)`.

## Design Choices
- main.go es chico a propósito: todo lo HTTP/negocio vive en `internal/`. main solo cablea.
- Bootstrap usa un `initCtx` con timeout de 5s para evitar colgarse si Postgres o Redis no contestan.
- `defer db.Close()` y `defer cacheClient.Close()` corren incluso si server.Run termina por error.

## Not Here
Buscando rutas, middlewares, lógica de handlers o de transcoding: están en
`internal/server`, `internal/api`, `internal/handlers`, `internal/transcode`,
`internal/storage`, `internal/cache`, `internal/process`. Ver `backend/CONTEXT_CAPSULE.md`
para el panorama completo del paquete.
