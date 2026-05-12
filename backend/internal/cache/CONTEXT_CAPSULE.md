# CONTEXT CAPSULE - internal/cache

## Purpose
Wrapper minimalista sobre Redis (`github.com/redis/go-redis/v9`) para cache-aside del backend. Bajo `internal/` para evitar imports externos.

## Files
- `cache.go`
  - `Client` envuelve `*redis.Client` con semántica fail-soft.
  - `New(ctx, redisURL)` — si la URL está vacía, es inválida o el ping falla, devuelve un `Client` con `rdb=nil` y loggea el motivo. No retorna error.
  - `GetJSON(ctx, key, dest)` — devuelve `false` en miss, error de red o unmarshal fallido.
  - `SetJSON(ctx, key, value, ttl)` — marshalea y setea con TTL; loggea y sigue si falla.
  - `Del(ctx, keys...)` — borra una o varias claves.
  - `Close()` — idempotente.
  - `IsConnected()` — `true` si hay conexión Redis viva; `false` en modo fail-soft. Lo consume `/health` para reportar `redis_up`.

## Fail-soft Contract
Si `rdb` es `nil`, todas las operaciones son no-op:
- `GetJSON` devuelve `false` (miss) → caller cae al storage.
- `SetJSON`/`Del` no hacen nada.

Esto permite que el backend siga funcionando sin Redis (POC). No usar este paquete para datos que requieran consistencia fuerte.

## Key Helpers
- `KeyVideoList = "videos:list"`
- `KeyStreamList = "streams:list"`
- `KeyVideo(id)` → `"videos:{id}"`
- `KeyStream(id)` → `"streams:{id}"`

## Usage Pattern (cache-aside)
1. `if cacheClient.GetJSON(ctx, key, &dest) { return dest }`
2. Leer de storage.
3. `cacheClient.SetJSON(ctx, key, dest, ttl)`.
4. En toda mutación, `cacheClient.Del(ctx, listKey, itemKey)`.

TTLs vigentes en `cmd/main.go`: 30s para listas, 60s para ítems individuales.

## Design Choices
- JSON como encoding (no msgpack/proto): debug fácil con `redis-cli GET`.
- Sin prefijo de namespace: una sola app por instancia Redis.
- Sin Pipeline/MGET: volumen actual no lo justifica.
- Sin métricas (hit/miss): añadir cuando haya observabilidad real.

## Known Gaps
- Sin tests.
- Sin pub/sub para invalidación distribuida (no hay réplicas del backend todavía).
- `SetJSON` no propaga errores; un fallo de Redis solo deja log.
