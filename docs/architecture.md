# Helmsman — Architecture

## Overview

Helmsman is a self-hosted, OpenAI-compatible LLM inference gateway. It routes
inference requests across multiple model backends with load balancing, per-key
rate limiting, semantic caching, circuit breaking, and full Prometheus
observability. Deployed on Kubernetes with horizontal autoscaling.

---

## Request Lifecycle

```
Client (OpenAI SDK / curl)
         │
         │  POST /v1/chat/completions
         │  Authorization: Bearer <key>
         ▼
┌─────────────────────────────────────────┐
│              Middleware Chain            │
│                                          │
│  1. RecoverMiddleware                    │
│     └─ catches panics, returns 500       │
│                                          │
│  2. RequestIDMiddleware                  │
│     └─ attaches X-Request-ID header      │
│                                          │
│  3. AuthMiddleware                       │
│     └─ extracts Bearer token → 401       │
│                                          │
│  4. RateLimitMiddleware                  │
│     └─ token bucket in Redis → 429       │
│                                          │
│  5. Semaphore (backpressure)             │
│     └─ bounded concurrency → 503         │
│                                          │
│  6. LoggingMiddleware                    │
│     └─ structured JSON log + Prometheus  │
└──────────────────┬──────────────────────┘
                   │
                   ▼
         ┌─────────────────┐
         │ ChatCompletions  │
         │    Handler       │
         └────────┬────────┘
                  │
         ┌────────▼────────┐
         │  Semantic Cache  │  embed prompt → cosine sim ≥ 0.95 → HIT
         │  Lookup (Redis)  │
         └────────┬────────┘
              miss│
                  ▼
         ┌─────────────────┐
         │  Round-Robin     │
         │  Load Balancer   │  picks from healthy backend set
         └────────┬────────┘
                  │
         ┌────────▼────────┐
         │ Circuit Breaker  │  Closed / Open / Half-Open per backend
         │   + Retry        │  exponential backoff + jitter, diff backend
         └────────┬────────┘
                  │
       ┌──────────┴──────────┐
       ▼                     ▼
  Backend A            Backend B ...    (Ollama instances)
                  │
         ┌────────▼────────┐
         │  Cache Store     │  async, detached context.Background()
         │  (Redis vector)  │
         └─────────────────┘
```

---

## Components

### Gateway Core (`internal/server`)
HTTP server on Go's standard `net/http`. Middleware composed as a chain of
`func(http.Handler) http.Handler`. `WriteTimeout: 0` allows SSE streaming
without deadline errors.

### Backend Registry (`internal/registry`)
Tracks backend URLs, health state, and a circuit breaker per backend. A
background goroutine probes each backend's `/` every 5 seconds. Cancelled via
context on graceful shutdown.

### Load Balancer (`internal/balancer`)
Round-robin over the healthy set via `atomic.Uint64` — lock-free and safe for
concurrent gateway replicas.

### Rate Limiter (`internal/ratelimit`)
Token bucket per API key as a Redis Lua script — atomic across replicas in a
single round-trip. Stores token count and last-refill timestamp per key.

### Request Queue (`internal/queue`)
Buffered channel as a semaphore. When full, new requests get a 503 immediately.
Shedding load fast keeps p99 bounded under overload.

### Semantic Cache (`internal/cache`)
1. Embed the prompt with `nomic-embed-text` (768-dim vector).
2. Scan stored vectors, compute cosine similarity.
3. `similarity ≥ 0.95` → return cached response (HIT).
4. Otherwise forward, store embedding + response async.

### Circuit Breaker (`internal/resilience`)
Closed → Open (3 failures) → Half-Open (30s cooldown, 1 probe) → Closed.
Retries try different backends with `100ms * attempt + rand(50ms)` backoff.

### Metrics (`internal/metrics`)
Prometheus via `promauto`. Key metrics: request latency histogram, request
counter, cache hit/miss, rate limit rejections, backend health gauge, circuit
breaker state gauge.

---

## Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Language | Go | Goroutines map directly onto the concurrency model. Static binary, ~10MB image. |
| Embeddings | Ollama `nomic-embed-text` | Free, local, no API key. 768-dim, good quality. |
| Cache similarity | Cosine similarity | Direction-invariant, normalizes for prompt length. Threshold 0.95 balances precision vs recall. |
| Rate limit atomicity | Redis Lua script | Single round-trip, atomic across replicas. INCR+TTL is racy; WATCH/MULTI adds RTTs. |
| Backpressure | Immediate 503 | Queuing grows latency unboundedly. Shedding fast keeps p99 predictable. |
| Circuit breaker | Custom (~60 lines) | No external dependency. Trivial to explain and own in an interview. |
| Observability | Prometheus + Grafana | Industry standard scraping pattern. |
| Deployment | Kubernetes + HPA | Liveness/readiness probes, rolling updates, autoscaling. |

---

## Known Limitations and Production Extensions

| Limitation | Production Fix |
|---|---|
| Cache scan is O(n) with `KEYS` | Redis Stack HNSW vector index for O(log n) |
| Single Redis instance | Redis Sentinel or Cluster for HA |
| No persistent key store | Postgres table with per-key rate limits |
| Streaming not cached | Buffer stream, store, replay for hits |
