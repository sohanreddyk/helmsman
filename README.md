# Helmsman

A self-hosted, OpenAI-compatible **distributed LLM inference gateway**. Helmsman
routes inference requests across multiple model backends with load balancing,
per-key rate limiting, semantic caching, and circuit breaking, and ships with
Prometheus/Grafana observability and a Kubernetes deployment.

> Status: **Phase 1 — Basic Gateway** (streaming proxy, middleware chain,
> health checks, graceful shutdown).

## Architecture (target)

```
Client (OpenAI SDK)
   │  POST /v1/chat/completions
   ▼
Helmsman gateway
   requestID → auth → rate limit → semantic cache
        │                              │ hit → Redis
        ▼                              │
   load balancer (healthy backend)     │
        │                              │
   circuit breaker + retry             │
        ▼
   Ollama / vLLM backends  ×N
```

## Requirements

- Go 1.22+
- [Ollama](https://ollama.com) running locally as a backend

## Quick start

```bash
# 1. start a backend
ollama serve
ollama pull llama3.2:1b

# 2. run the gateway
make run          # or: go run ./cmd/gateway
```

The gateway listens on `:8080` by default.

## Configuration

| Env var                | Default                  | Description              |
|------------------------|--------------------------|--------------------------|
| `HELMSMAN_ADDR`        | `:8080`                  | Listen address           |
| `HELMSMAN_BACKEND_URL` | `http://localhost:11434` | Upstream backend base URL|

## Try it

```bash
curl -s localhost:8080/healthz

curl -s localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3.2:1b","messages":[{"role":"user","content":"say hi in 3 words"}]}'

# streaming
curl -N localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"llama3.2:1b","stream":true,"messages":[{"role":"user","content":"count to 5"}]}'
```

## Endpoints

| Method | Path                    | Description                      |
|--------|-------------------------|----------------------------------|
| POST   | `/v1/chat/completions`  | OpenAI-compatible chat endpoint  |
| GET    | `/healthz`              | Gateway liveness                 |
| GET    | `/readyz`               | Readiness (real check in Phase 2)|

## Roadmap

1. **Basic gateway** — streaming proxy, middleware, health, graceful shutdown ✅
2. Multi-backend routing — registry, health probes, round-robin
3. Rate limiting — Redis token buckets per API key
4. Request queue — bounded concurrency + backpressure
5. Semantic cache — embeddings + cosine match
6. Circuit breakers + retries
7. Metrics + dashboard — Prometheus/Grafana + React
8. Docker + Kubernetes
9. Docs + interview story
