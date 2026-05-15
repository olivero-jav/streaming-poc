# syntax=docker/dockerfile:1.7

# ---------------------------------------------------------------------------
# Stage 1: build Angular frontend
# ---------------------------------------------------------------------------
FROM node:22-alpine AS frontend-builder
WORKDIR /build

# Install deps first (cached as long as package files don't change).
COPY frontend/streaming-frontend/package.json frontend/streaming-frontend/package-lock.json ./
RUN npm ci

# Bring in sources and build. Output: /build/dist/streaming-frontend/browser/
COPY frontend/streaming-frontend/ ./
RUN npm run build

# ---------------------------------------------------------------------------
# Stage 2: build Go backend
# ---------------------------------------------------------------------------
FROM golang:1.25-alpine AS backend-builder
WORKDIR /build/backend

# Module download cached separately from sources.
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags='-s -w' -o /out/streaming-backend ./cmd

# ---------------------------------------------------------------------------
# Stage 3: runtime image
# ---------------------------------------------------------------------------
FROM alpine:3.20

# ffmpeg is required at runtime for VOD transcoding and live HLS capture.
# wget is used by the healthcheck. ca-certificates + tzdata for HTTPS/log timestamps.
RUN apk add --no-cache ffmpeg ca-certificates tzdata wget \
    && addgroup -S app \
    && adduser -S -G app app

# The SPA fallback in handlers/spa.go expects the Angular dist at:
#   $BACKEND_ROOT/../frontend/streaming-frontend/dist/streaming-frontend/browser
# Mirror the repo layout so that relative path resolves correctly.
WORKDIR /app/backend

COPY --from=backend-builder /out/streaming-backend /app/backend/streaming-backend
COPY --from=frontend-builder /build/dist/streaming-frontend/browser \
    /app/frontend/streaming-frontend/dist/streaming-frontend/browser

# Writable dirs for uploaded sources and generated HLS output.
RUN mkdir -p /app/backend/uploads /app/backend/media \
    && chown -R app:app /app

USER app

ENV BACKEND_ROOT=/app/backend
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O - http://127.0.0.1:8080/health || exit 1

CMD ["/app/backend/streaming-backend"]
