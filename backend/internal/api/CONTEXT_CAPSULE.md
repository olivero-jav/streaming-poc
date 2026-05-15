# CONTEXT CAPSULE - internal/api

## Purpose
Middlewares y helpers HTTP transversales que no encajan en `handlers/` (porque no son endpoints) ni en `server/` (porque no son wiring). Cubre CORS, security headers, límite de upload y detección de MIME.

## Files
- `cors.go` — `CORS(allowedOrigins, allowNgrok)` devuelve un gin middleware con allow-list explícita. `allowNgrok` permite además cualquier `https://*.ngrok-free.app` o `https://*.ngrok.io` (parseando el origin con `url.Parse`, no concatenando strings). Responde `204` a preflight `OPTIONS`. Tests en `cors_test.go`.
- `security.go` — `SecurityHeaders()` middleware seteando `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`, `X-Frame-Options: DENY` y `Strict-Transport-Security` solo cuando la request es HTTPS (incluye `X-Forwarded-Proto: https` para detectar ngrok/proxy). El CSP **no** se setea acá — solo aplica a documentos HTML, así que lo agrega el handler SPA. `ContentSecurityPolicy()` exporta el string para que `handlers/spa.go` lo use. Tests en `security_test.go`.
- `upload.go` — `LimitUploadSize(limit)` envuelve `c.Request.Body` con `http.MaxBytesReader`, así un upload mayor falla con `*http.MaxBytesError` que el handler traduce a 413. `DetectVideoMime(fileHeader)` abre el `multipart.FileHeader`, lee los primeros 512 bytes y los pasa por `http.DetectContentType` — más confiable que confiar en la extensión o en el `Content-Type` del multipart, ambos spoofeables. `AllowedVideoMimes` es el set explícito: `video/mp4`, `video/webm`, `video/quicktime`. MKV excluido porque el stdlib detecta inconsistentemente (suele devolver `application/octet-stream`).

## CSP rationale
El CSP que retorna `ContentSecurityPolicy()` permite `'unsafe-inline'` en `script-src` y `style-src` porque Angular CLI emite un onload inline para cargar CSS async, y Beasties inlinea critical CSS. `media-src blob:` y `worker-src blob:` los necesita hls.js para `MediaSource` y su parser worker. Se puede tightenar (nonces) si se introduce SSR.

## Notas
- Ningún archivo acá habla con DB ni cache; es presentation-layer pura.
- Los middlewares son aplicados por `server.Build` en este orden: CORS → gzip → SecurityHeaders → rutas. `LimitUploadSize` se aplica solo a `POST /videos`.
