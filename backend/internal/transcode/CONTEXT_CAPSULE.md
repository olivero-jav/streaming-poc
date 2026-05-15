# CONTEXT CAPSULE - internal/transcode

## Purpose
Owns el trabajo backed-by-ffmpeg: transcoding VOD y captura RTMP→HLS para Live. La capa HTTP dispatchea acá desde goroutines de background (`Deps.BgRun`) y nunca bloquea directamente esperando ffmpeg.

## Files
- `vod.go` — `ProcessVideo(appCtx, db, cache, backendRoot, videoID, sourcePath, sem)`. Adquiere el semáforo, marca `processing`, lanza ffmpeg (libx264 + AAC, HLS event playlist, segmentos de 6s) con `exec.CommandContext(appCtx, ...)`. Poll en goroutine separada: cuando aparece `index0.ts`, setea `hls_path` y borra cache (el cliente puede empezar a reproducir mientras el transcoding sigue). Al terminar exitosamente, `MarkVideoReady`. En cualquier error, `markVideoError`. Usa "fresh context" de 5s para los UPDATE de cierre, para que lleguen incluso si `runCtx` fue cancelado por shutdown.
- `live.go` — `StartLive(appCtx, db, cache, backendRoot, stream, registry)`. Lanza ffmpeg que lee `rtmp://localhost:1935/live/{streamKey}` y escribe HLS (copy video, AAC audio). Registra el `cmd` en el `process.Registry` para que el hook unpublish pueda matarlo. Poll del primer segmento con deadline de 30s; cuando aparece, `MarkStreamLive`. Cuando ffmpeg termina, `MarkStreamEnded` con fresh context, y dispara `promoteStreamToVOD` salvo que estemos en shutdown.
- `live.go::promoteStreamToVOD` — crea un VOD `ready` apuntando a los archivos HLS live in-situ (sin retranscoding ni copia). Llama a `finalizeHLSPlaylist` para appendear `#EXT-X-ENDLIST` si ffmpeg fue killado y no lo cerró limpiamente.
- `buffer.go` — `lockedBuffer`: wrapper de `bytes.Buffer` con mutex. ffmpeg escribe a stdout y stderr desde goroutines distintas; cuando ambos apuntan al mismo sink, el sink debe serializar. Solo lo usa `vod.go` (live manda stderr directo a `os.Stderr`).
- `buffer_test.go` — verifica concurrencia segura.

## Concurrencia
- VOD: semáforo `TranscodeSem` (capacidad 4) bloquea hasta poder transcodificar. Cancela por `appCtx` si llega shutdown antes de adquirir.
- Live: sin semáforo; cada stream arranca cuando MediaMTX lo notifica.
- Ambos heredan de `appCtx`: ffmpeg muere cuando el proceso muere.
- VOD agrega un `runCtx` con timeout de 30 minutos derivado de `appCtx`.

## Cache touches
- VOD: invalida `videos:list` + `videos:{id}` al publicar `hls_path` (primer segmento) y al marcar `ready`/`error`.
- Live: invalida `streams:list` + `streams:{id}` al pasar a `live` y al terminar.
- VOD promovido desde live invalida `videos:list`.

## Gaps conocidos
- `promoteStreamToVOD` se spawnea como `go` desnudo (no trackeada por `BgWG`). El check `appCtx.Err() == nil` mitiga pero tiene una ventana de race entre el check y el spawn.
- Duplicación del polling-primer-segmento entre `vod.go` y `live.go`. Candidato a helper compartido o a `fsnotify`.
- Sin métricas (duración de transcoding, fallos, throughput).
