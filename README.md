# Helmsman

A self-hosted, OpenAI-compatible **distributed LLM inference gateway** written
in Go. Routes inference requests across multiple model backends with load
balancing, per-key rate limiting, semantic caching, circuit breaking, and
Prometheus/Grafana observability. Deployed on Kubernetes with HPA.

---

## Benchmark results

| Scenario | p50 | p95 | p99 | RPS |
|---|---|---|---|---|
| Cache MISS (cold, Ollama M-series Mac) | 32ms | 104ms | — | 34 |
| Cache HIT (warm, served from Redis) | **0.2ms** | 127ms | 166ms | **594** |

Cache hits are **17x faster** at p50 and deliver **17x more throughput** than cold
requests. The p95/p99 spread on warm is from the rate limiter returning 429s
instantly — actual hit latency is sub-millisecond.

Load tested with [`hey`](https://github.com/rakyll/hey) on a MacBook Air M-series,
single Ollama backend (`llama3.2:1b`), Redis 7 on Docker.

---

## What this demonstrates

| Area | What Helmsman shows |
|---|---|
| Backend engineering | Streaming proxy, middleware chain, graceful shutdown |
| Distributed systems | Rate limiting across replicas (atomic Lua), backpressure, circuit breakers |
| AI infrastructure | OpenAI-compatible API surface, semantic caching with embeddings |
| Observability | Prometheus histograms, Grafana dashboards, structured JSON logging |
| Kubernetes | Liveness/readiness probes, HPA, rolling updates, zero-downtime failover |

---

## Architecture

```
Client
  │  POST /v1/chat/completions
  ▼
recover → requestID → auth → rate limit → semaphore → logging
  │
  ▼
semantic cache (embed → cosine sim ≥ 0.95 → HIT, ~0.2ms)
  │ miss
  ▼
round-robin load balancer (healthy backends only)
  │
  ▼
circuit breaker + retry (exponential backoff, different backend)
  │
  ▼
Ollama backends ×N
  │
  ▼
async cache store (Redis vector, context.Background())
```

See [docs/architecture.md](docs/architecture.md) for the full design doc including
all design decisions and known production limitations.

---

## Stack

| Layer | Technology |
|---|---|
| Gateway | Go 1.24, standard `net/http` |
| Backends | Ollama (local LLM runtime) |
| Embeddings | `nomic-embed-text` via Ollama (768-dim) |
| Cache / Rate limit | Redis 7 |
| Metrics | Prometheus + Grafana |
| Dashboard | Vanilla React (no build step) |
| Deployment | Docker + Kubernetes (Docker Desktop) |

---

## Quick start

```bash
# Prerequisites: Go 1.22+, Ollama, Docker

ollama pull llama3.2:1b
ollama pull nomic-embed-text

docker run -d --name helmsman-redis -p 6379:6379 redis:7-alpine

make run
```

Gateway listens on `:8080`.

```bash
curl -s localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer mykey" \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3.2:1b","messages":[{"role":"user","content":"say hi"}]}'
```

---

## Live dashboard

```bash
make dashboard   # opens http://localhost:8081
```

Shows backend health, cache hit rate, RPS sparkline, circuit breaker state,
and lifetime counters — polling `/stats` and `/metrics` every 2 seconds.

---

## Configuration

| Env var | Default | Description |
|---|---|---|
| `HELMSMAN_ADDR` | `:8080` | Listen address |
| `HELMSMAN_BACKEND_URLS` | `http://localhost:11434` | Comma-separated backend URLs |
| `HELMSMAN_REDIS_ADDR` | `localhost:6379` | Redis address |
| `HELMSMAN_EMBED_URL` | `http://localhost:11434` | Ollama base URL for embeddings |

Copy `.env.example` to `.env` and adjust for your setup.

---

## Endpoints

| Method | Path | Description |
|---|---|---|
| POST | `/v1/chat/completions` | OpenAI-compatible inference |
| GET | `/healthz` | Gateway liveness (always 200) |
| GET | `/readyz` | Readiness (503 if no healthy backends) |
| GET | `/metrics` | Prometheus exposition |
| GET | `/stats` | JSON snapshot for dashboard |

---

## Observability

```bash
cd deploy && docker compose up -d
```

- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000 (admin/admin) — add Prometheus datasource at `http://prometheus:9090`

Key PromQL queries:
```promql
rate(helmsman_requests_total[1m])
histogram_quantile(0.99, rate(helmsman_request_duration_seconds_bucket[1m]))
helmsman_cache_hits_total / (helmsman_cache_hits_total + helmsman_cache_misses_total)
helmsman_backend_healthy
helmsman_circuit_breaker_state
```

---

## Kubernetes

```bash
make docker-build
make k8s-up
make k8s-status
```

Deploys 2 Helmsman replicas + Redis into the `helmsman` namespace, with:
- HPA scaling 2→5 replicas at 60% CPU
- Liveness probe on `/healthz`
- Readiness probe on `/readyz` — Kubernetes removes pods from LB when no backends are healthy
- `NodePort` on `30080` for local access

---

## Testing

```bash
go test ./...
```

Covers: cosine similarity correctness, circuit breaker state machine (all
transitions), round-robin distribution, semaphore backpressure + health endpoint
bypass

---

## Build roadmap

- [x] Phase 1 — Streaming proxy, middleware chain, graceful shutdown
- [x] Phase 2 — Backend registry, health probes, round-robin LB
- [x] Phase 3 — Redis token-bucket rate limiting per API key (atomic Lua)
- [x] Phase 4 — Bounded concurrency + immediate backpressure
- [x] Phase 5 — Semantic cache with embeddings + cosine similarity
- [x] Phase 6 — Per-backend circuit breakers + exponential backoff retry
- [x] Phase 7 — Prometheus metrics + Grafana + `/stats` endpoint
- [x] Phase 8 — Docker + Kubernetes + HPA
- [x] Phase 9 — Architecture docs, load tests, live dashboard, test suite
