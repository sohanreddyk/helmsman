# Helmsman

A self-hosted, OpenAI-compatible **distributed LLM inference gateway** written
in Go. Routes inference requests across multiple model backends with load
balancing, per-key rate limiting, semantic caching, circuit breaking, and
Prometheus/Grafana observability. Deployed on Kubernetes with HPA.

---

## What this demonstrates

| Area | What Helmsman shows |
|---|---|
| Backend engineering | Streaming proxy, middleware chain, graceful shutdown |
| Distributed systems | Rate limiting across replicas (atomic Lua), backpressure, circuit breakers |
| AI infrastructure | OpenAI-compatible API surface, semantic caching with embeddings |
| Observability | Prometheus histograms, Grafana dashboards, structured logging |
| Kubernetes | Liveness/readiness probes, HPA, rolling updates, failover |

---

## Architecture

```
Client
  │  POST /v1/chat/completions
  ▼
recover → requestID → auth → rate limit → semaphore → logging
  │
  ▼
semantic cache (embed → cosine sim ≥ 0.95 → HIT)
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
async cache store (Redis vector)
```

See [docs/architecture.md](docs/architecture.md) for the full design doc.

---

## Stack

| Layer | Technology |
|---|---|
| Gateway | Go 1.24, standard `net/http` |
| Backends | Ollama (local LLM runtime) |
| Embeddings | `nomic-embed-text` via Ollama |
| Cache / Rate limit | Redis 7 |
| Metrics | Prometheus + Grafana |
| Deployment | Docker + Kubernetes (Docker Desktop) |

---

## Quick start

```bash
# Prerequisites: Go 1.22+, Ollama, Docker, Kubernetes

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

## Configuration

| Env var | Default | Description |
|---|---|---|
| `HELMSMAN_ADDR` | `:8080` | Listen address |
| `HELMSMAN_BACKEND_URLS` | `http://localhost:11434` | Comma-separated backend URLs |
| `HELMSMAN_REDIS_ADDR` | `localhost:6379` | Redis address |
| `HELMSMAN_EMBED_URL` | `http://localhost:11434` | Ollama base URL for embeddings |

---

## Endpoints

| Method | Path | Description |
|---|---|---|
| POST | `/v1/chat/completions` | OpenAI-compatible inference |
| GET | `/healthz` | Gateway liveness |
| GET | `/readyz` | Readiness (≥1 healthy backend) |
| GET | `/metrics` | Prometheus exposition |
| GET | `/stats` | JSON snapshot for dashboard |

---

## Kubernetes deployment

```bash
make docker-build
make k8s-up
make k8s-status
```

Deploys 2 gateway replicas + Redis, with HPA scaling 2→5 on CPU, and
liveness/readiness probes wired to `/healthz` and `/readyz`.

---

## Observability

Start Prometheus + Grafana:

```bash
cd deploy && docker compose up -d
```

- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin)

Key PromQL queries:
```
rate(helmsman_requests_total[1m])
histogram_quantile(0.99, rate(helmsman_request_duration_seconds_bucket[1m]))
helmsman_cache_hits_total / (helmsman_cache_hits_total + helmsman_cache_misses_total)
helmsman_backend_healthy
```

---

## Measured results

| Metric | Value |
|---|---|
| Cache miss latency | ~1.3s (Ollama on M-series Mac) |
| Cache hit latency | ~160ms (8x improvement) |
| Rate limit enforcement | Atomic across replicas via Redis Lua |
| Failover | Automatic — dead backend isolated within 5s health probe cycle |
| Pod restart recovery | ~10s (Kubernetes reschedule) |

---

## Build roadmap

- [x] Phase 1 — Streaming proxy, middleware chain, graceful shutdown
- [x] Phase 2 — Backend registry, health probes, round-robin LB
- [x] Phase 3 — Redis token-bucket rate limiting per API key
- [x] Phase 4 — Bounded concurrency + backpressure
- [x] Phase 5 — Semantic cache with embeddings + cosine similarity
- [x] Phase 6 — Circuit breakers + retry with exponential backoff
- [x] Phase 7 — Prometheus metrics + Grafana + /stats endpoint
- [x] Phase 8 — Docker + Kubernetes + HPA
- [x] Phase 9 — Architecture docs + interview story
