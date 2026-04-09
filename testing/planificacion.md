# Planificacion inicial de testing de carga

## Objetivo

Validar la capacidad real del proyecto por escalas (10, 50, 100, 500, 1k usuarios) sin depender de ngrok para las pruebas de carga.

## Alcance

- Backend API (Go): `/health`, `/videos`, `/videos/:id`, `/streams`, `/streams/:id`
- Entrega HLS: playlists y segmentos en VOD y Live
- Ingest RTMP via MediaMTX + FFmpeg
- Recursos del host: CPU, memoria, disco, red

## Entorno de prueba

- Ejecutar en red local (localhost/IP privada), no sobre ngrok para carga alta.
- Backend corriendo en `:8080`.
- MediaMTX corriendo con `docker compose up -d`.
- Dataset de prueba estable (videos y streams de prueba).

## Matriz de ejecucion por fase


| Fase | k6 VUs (API+HLS) | Publishers FFmpeg | Duracion            | Mix trafico       | Criterio pasar                                    |
| ---- | ---------------- | ----------------- | ------------------- | ----------------- | ------------------------------------------------- |
| 10   | 10               | 1                 | 15 min              | 70% HLS / 30% API | error < 0.5%, p95 < 250 ms, sin buffering visible |
| 50   | 50               | 2-3               | 20 min              | 75% HLS / 25% API | error < 1%, p95 < 300 ms, CPU < 70%               |
| 100  | 100              | 5                 | 30 min              | 80% HLS / 20% API | error < 1%, p95 < 400 ms, rebuffer < 2%           |
| 500  | 500              | 8-10              | 30 min + pico 5 min | 85% HLS / 15% API | error < 2%, p95 < 700 ms, sin caidas              |
| 1k   | 1000             | 10-15             | 20 min + pico 2 min | 90% HLS / 10% API | error < 3%, p95 < 900 ms, estabilidad aceptable   |


## Metricas obligatorias por corrida

- API: p50, p95, p99, error rate, RPS
- HLS: exito de `.m3u8` y `.ts`, TTFF, rebuffer ratio
- Sistema: CPU, memoria, I/O disco, ancho de banda
- Procesos: estado de FFmpeg y errores de backend/MediaMTX

## Regla operativa

- Verde: cumple objetivos, avanzar a siguiente escala.
- Amarillo: cerca del limite, repetir fase con ajustes menores.
- Rojo: supera umbrales, detener escalado y analizar cuello de botella.
- Warmup obligatorio: 3-5 minutos al 30% de la carga objetivo antes de cada fase.
- No avanzar si la fase falla en 2 mediciones consecutivas.

## Registro minimo por corrida

- Fase ejecutada, fecha y duracion real.
- p50/p95/p99, error rate y RPS (`k6`).
- Exito de `.m3u8` y `.ts`, TTFF y rebuffer.
- CPU, memoria, disco y red durante la corrida.
- Decision final: aprobado, aprobado con riesgo o no aprobado.

## Definicion: `api-smoke`

### Objetivo

Validar rapidamente que la API soporta carga ligera/moderada sin errores funcionales antes de correr escenarios mas pesados.

### Endpoints incluidos

- `GET /health`
- `GET /videos`
- `GET /streams`
- `GET /videos/:id` (tomando id desde respuesta de `/videos`)
- `GET /streams/:id` (tomando id desde respuesta de `/streams`)

### Perfil de trafico (mix)

- 10% `GET /health`
- 35% `GET /videos`
- 25% `GET /streams`
- 15% `GET /videos/:id`
- 15% `GET /streams/:id`

### Parametros de ejecucion por fase

- Fase 10: `vus=10`, `duration=15m`
- Fase 50: `vus=50`, `duration=20m`
- Fase 100: `vus=100`, `duration=30m`

Nota: `api-smoke` no cubre HLS ni RTMP; sirve como gate previo.

### Thresholds iniciales

- `http_req_failed < 1%` (en fase 10 ideal < 0.5%)
- `http_req_duration p(95) < 300 ms` (fase 10/50) y `< 400 ms` (fase 100)
- `checks > 99%`

### Criterio de aprobado

- No hay errores funcionales (respuestas 200 esperadas en lecturas).
- Cumple thresholds de latencia y error.
- No hay crecimiento anomalo de memoria del backend durante la corrida.

### Salida esperada de la prueba

- Resumen `k6` con p50/p95/p99, `http_req_failed`, `http_reqs`.
- Registro manual de CPU/RAM promedio del host.
- Decision: aprobar y pasar a `hls-vod`, o repetir con ajustes.

## Definicion: `hls-vod`

### Objetivo

Simular muchos espectadores concurrentes que piden playlist y segmentos de un video VOD ya transcodificado, midiendo capacidad de lectura (disco + servidor HTTP) sin escritura de nuevos segmentos.

### Prerrequisitos

- Un `VIDEO_ID` en estado `ready` con HLS generado bajo `/hls/vod/:id/`.
- Verificar manualmente una vez: `GET /hls/vod/{VIDEO_ID}/index.m3u8` y al menos un segmento listado devuelve `200`.

### URLs bajo prueba

- `GET /hls/vod/{VIDEO_ID}/index.m3u8`
- `GET /hls/vod/{VIDEO_ID}/{segmento}` (nombres reales obtenidos del `.m3u8` en cada iteracion; no hardcodear nombres fijos salvo smoke manual)

### Perfil de trafico (mix sugerido)

- 15% solo playlist (refresco frecuente del manifest).
- 85% playlist + descarga de N segmentos por iteracion (N=3 a 6), parseando el `.m3u8` para lineas que no empiezan por `#` y no son la propia URL absoluta del manifest.

### Parametros de ejecucion por fase

- Alinear VUs y duracion con la matriz global; este escenario cuenta como parte del porcentaje HLS del mix (70-90% segun fase).
- Fase 10: `vus=10`, `duration=15m` (solo `hls-vod` o combinado con `api-smoke` en otro proceso segun matriz).
- Fases superiores: mismos VUs objetivo que la fase, proporcion HLS segun tabla.

### Thresholds iniciales

- `http_req_failed < 1%` (fase 10 ideal < 0.5%).
- `http_req_duration p(95)`: playlist < 400 ms (fases 10-100); segmentos suelen ser mas lentos: p95 < 800 ms (10-50), < 1200 ms (100), ajustar si el bitrate es alto.
- `checks > 99%` en validacion de status 200 y cuerpo no vacio en playlist.

### Criterio de aprobado

- Ratio de `404` en segmentos despreciable frente al total de requests.
- Sin saturacion sostenida de disco (latencia de lectura estable en el host).
- Si se combina con `api-smoke`, ambos escenarios cumplen sus thresholds en la misma ventana o se documenta degradacion cruzada.

### Salida esperada

- Metricas `k6` separadas por nombre de request (playlist vs segment) si el script las etiqueta.
- Nota de tamano medio de segmento y RPS de bytes si se habilita resumen en `k6`.

---

## Definicion: `hls-live`

### Objetivo

Simular espectadores concurrentes sobre un stream en vivo: playlists que se actualizan y segmentos nuevos mientras FFmpeg escribe, midiendo lectura concurrente bajo escritura continua.

### Prerrequisitos

- `STREAM_ID` con estado `live` (u operacion equivalente en tu flujo).
- Al menos un publisher RTMP activo hacia MediaMTX con el `stream_key` correcto (OBS o `ffmpeg`).
- Backend generando HLS en `/hls/live/{STREAM_ID}/` durante toda la prueba.

### URLs bajo prueba

- `GET /hls/live/{STREAM_ID}/index.m3u8`
- `GET /hls/live/{STREAM_ID}/{segmento}` (uris relativas del manifest actual, parseadas en cada iteracion).

### Perfil de trafico (mix sugerido)

- 25% refresco rapido de playlist (cada 1-2 s simulado con `sleep` corto en un subconjunto de VUs o peso en el script).
- 75% playlist + descarga de los ultimos K segmentos del manifest (K=2 a 4), priorizando lineas `#EXTINF` recientes para aproximar comportamiento de player.

### Publishers FFmpeg (alineado con matriz)

- Fase 10: 1 publisher.
- Fase 50: 2-3 publishers en streams distintos si se quiere medir multi-stream; si solo hay un `STREAM_ID` live, mantener 1 publisher y escalar solo viewers.
- Fases 100+: segun columna "Publishers FFmpeg" de la matriz; cada publisher adicional incrementa CPU y I/O — documentar cuantos streams live hay activos durante la corrida.

### Parametros de ejecucion por fase

- Mismos VUs y duraciones que la matriz para la parte HLS.
- No iniciar `k6` hasta confirmar playlist `200` y al menos un segmento `200` con el publisher ya estable.

### Thresholds iniciales

- `http_req_failed < 1.5%` (live tolera mas variacion por condiciones de carrera playlist/segmento que VOD).
- Playlist p95 < 500 ms (10-50), < 800 ms (100+).
- Segmentos p95 algo mayor que VOD por contencion con escritura; documentar baseline en fase 10 antes de fijar numeros duros en 500/1k.
- Si aparecen `404` esporadicos en el segmento "mas nuevo", medir frecuencia; falla si supera ~2% de requests de segmentos.

### Criterio de aprobado

- El manifest se sirve de forma estable durante toda la ventana.
- No hay caida del proceso FFmpeg del backend ni reinicios en cadena visibles en logs.
- CPU/memoria del host dentro de los limites acordados para la fase.

### Salida esperada

- Mismo tipo de salida que `hls-vod`, mas anotacion de numero de publishers y duracion del live durante la prueba.
- Captura de cualquier patron de error temporal (segmento aun no escrito) para afinar `sleep` o reintentos en el script.

---

## Composicion de una corrida completa (por fase)

1. Warmup: `api-smoke` al 30% VUs durante 3-5 min (opcional pero recomendado).
2. `hls-vod` con dataset conocido (valida data plane estatico).
3. `hls-live` con publisher(es) activos (valida data plane dinamico + ingest).
4. Opcional: ejecutar `api-smoke` y escenarios HLS en paralelo con dos procesos `k6` para reproducir el mix de la matriz, ajustando VUs en cada script para sumar el total de la fase.

## Scripts `k6` (listos)

Archivos en `testing/`:

- `api-smoke.js`
- `hls-vod.js` — requiere `VIDEO_ID`
- `hls-live.js` — requiere `STREAM_ID`

### Ejecucion (PowerShell, desde `streaming-poc/`)

```powershell
$env:BASE_URL = "http://localhost:8080"

# API
$env:VUS = "10"
$env:DURATION = "15m"
k6 run testing/api-smoke.js

# VOD (sustituir UUID)
$env:VIDEO_ID = "..."
k6 run testing/hls-vod.js

# Live (stream en vivo + publisher RTMP activo)
$env:STREAM_ID = "..."
k6 run testing/hls-live.js
```

Variables opcionales para afinar thresholds: `THRESHOLD_FAILED`, `THRESHOLD_P95`, `THRESHOLD_P95_PLAYLIST` (ver comentarios al inicio de cada script).

Si `k6` no esta en PATH tras winget, usa la ruta completa a `k6.exe`.