# HLS latency probe

Mide `hls.latency` (distancia player↔edge según hls.js) en distintos escenarios de estrés via Playwright + CDP.

## Pre-requisitos

- Backend Go corriendo en `http://localhost:8080`.
- MediaMTX corriendo (RTMP en `:1935`).
- `ffmpeg` en el PATH.
- Frontend Angular sirviendo en `http://localhost:4200` (o build estatica servida por el backend).

## Uso

```bash
npm install
npx playwright install chromium
npm run run        # corre todos los escenarios
npm run report     # imprime resumen p50/p95
```

Output en `results/<timestamp>/<scenario>.csv` + `summary.json`.
