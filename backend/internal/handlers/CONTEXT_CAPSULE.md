# CONTEXT CAPSULE - internal/handlers

## Purpose
HTTP handlers del backend, agrupados por dominio. Reciben `*Deps` y devuelven `gin.HandlerFunc`, lo que mantiene el wiring (`server.Build`) desacoplado de la lógica de request y hace cada handler testeable con un `Deps` stub.

## Files
- `deps.go` — define `Deps` (DB, Cache, Registry, Cfg, AppCtx, TranscodeSem, BgWG) y el helper `BgRun` que spawnea goroutines trackeadas por `BgWG`, recibiendo `appCtx` para cancelación.
- `health.go` — `GET /health` → `{status, redis_up, commit}`.
- `videos.go` — `UploadVideo`, `ListVideos`, `GetVideo`, `UpdateVideoStatus`. Upload usa `ParseMultipartForm` explícito para que `MaxBytesReader` surfacee como 413 antes de que Gin enmascare el error como "title required". Valida MIME con magic bytes (`api.DetectVideoMime`).
- `streams.go` — `CreateStream`, `ListStreams`, `GetStream`.
- `hls.go` — `ServeHLSPlaylist` y `ServeHLSSegment` parametrizados por `baseSubdir` (`hls` o `live`), comparten `serveHLSFile`. Path traversal cubierto con `strings.Contains("..")` + `filepath.Clean` + `HasPrefix(baseDir)`. Cache-Control: playlists `max-age=2, public`, segmentos `max-age=31536000, immutable`. Solo sirven `.m3u8` y `.ts`.
- `hooks.go` — `PublishHook` y `UnpublishHook` para los webhooks de MediaMTX. Parsean `path=live/{streamKey}` y disparan transcode / kill.
- `spa.go` — `SPAFallback(distDir)` devuelve un handler para `NoRoute` que sirve el build de Angular. Devuelve `nil` si `distDir` no existe (dev local con Angular en 4200). Aplica CSP solo a respuestas HTML.

## Tests
- `videos_test.go` — cubre missing file, missing title (incl. whitespace), MIME no-video rechazado (415). Usa `multipartBody` helper para armar requests.
- `hooks_test.go` — paths inválidos (400), stream key desconocido (404), unpublish sobre key desconocido es no-op (200). El happy-path de publish spawnea ffmpeg en background — cubierto por e2e bajo `testing/hls-live`.
- `testhelper_test.go` — `newTestDB` (schema temporal en Postgres, skip si no hay DB), `newTestDeps` (Deps stub con cache en modo fail-soft). El helper de DB está duplicado con el de `internal/storage` a propósito: el código nota explícitamente que duplicar es preferible a promover un `internal/storagetest` para ~80 líneas.

## Patrones
- Handlers como factories: `func XyzHandler(d *Deps) gin.HandlerFunc { return func(c *gin.Context) {...} }`. El cierre captura `d` para no rearmar dependencias por request.
- Cache-aside en lecturas: `if d.Cache.GetJSON(...) { return cached }`; sino, leer de storage, `SetJSON` con TTL (30s listas, 60s ítems), responder.
- Invalidación explícita (`d.Cache.Del`) tras cada mutación. Los flujos de transcoding hacen su propio `Del` desde `internal/transcode/`.
- Errores que vienen como `sql.ErrNoRows` se detectan con `errors.Is` y se mapean a 404.
- Background work se lanza con `d.BgRun(func(ctx context.Context) {...})` — recibe `appCtx` y se trackea en `BgWG` para shutdown limpio.

## Security / Gaps
- `PATCH /videos/:id/status` sin auth. Aceptable en POC.
- `/internal/hooks/{publish,unpublish}` **sin auth ni filtro por loopback**. Solo se asume que vienen de MediaMTX en localhost. Si el puerto 8080 se expone (ngrok), cualquiera con un `stream_key` puede dispararlos. TODO: shared-secret header o middleware que filtre por `c.ClientIP()`.
- `serveHLSFile` revisa traversal después de `filepath.Clean`. Los handlers de playlist/segmento además bloquean `..` en los parámetros antes de pasarlos.
