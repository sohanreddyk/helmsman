package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"github.com/sohanreddy/helmsman/internal/balancer"
	"github.com/sohanreddy/helmsman/internal/cache"
	"github.com/sohanreddy/helmsman/internal/metrics"
	"github.com/sohanreddy/helmsman/internal/proxy"
	"github.com/sohanreddy/helmsman/internal/registry"
)

const maxRetries = 3

type Handlers struct {
	registry *registry.Registry
	balancer *balancer.RoundRobin
	proxy    *proxy.Proxy
	cache    *cache.SemanticCache
	log      *slog.Logger
}

func NewHandlers(
	reg *registry.Registry,
	bal *balancer.RoundRobin,
	p *proxy.Proxy,
	c *cache.SemanticCache,
	log *slog.Logger,
) *Handlers {
	return &Handlers{registry: reg, balancer: bal, proxy: p, cache: c, log: log}
}

func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, `{"status":"ok"}`)
}

func (h *Handlers) Readyz(w http.ResponseWriter, r *http.Request) {
	if len(h.registry.Healthy()) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, `{"status":"no healthy backends"}`)
		return
	}
	writeJSON(w, http.StatusOK, `{"status":"ready"}`)
}

// Stats returns a JSON snapshot of gateway state for the dashboard.
func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	type backendStat struct {
		URL     string `json:"url"`
		Healthy bool   `json:"healthy"`
		Breaker string `json:"breaker"`
	}
	var backends []backendStat
	for _, b := range h.registry.All() {
		state := b.Breaker.State()
		var stateNum float64
		switch state {
		case "open":
			stateNum = 1
		case "half-open":
			stateNum = 2
		}
		metrics.BackendHealthy.WithLabelValues(b.URL).Set(func() float64 {
			if b.Healthy {
				return 1
			}
			return 0
		}())
		metrics.CircuitBreakerState.WithLabelValues(b.URL).Set(stateNum)
		backends = append(backends, backendStat{
			URL:     b.URL,
			Healthy: b.Healthy,
			Breaker: state,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"backends": backends,
	})
}

func (h *Handlers) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, `{"error":"failed to read body"}`)
		return
	}

	prompt, stream := extractPrompt(body)

	// Semantic cache lookup
	if !stream && h.cache != nil && prompt != "" {
		if cached, hit, err := h.cache.Get(r.Context(), prompt); err == nil && hit {
			h.log.Info("cache hit", "prompt_len", len(prompt))
			metrics.CacheHits.Inc()
			w.Header().Set("X-Cache", "HIT")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cached)
			return
		}
		metrics.CacheMisses.Inc()
	}

	responseBytes, err := h.forwardWithRetry(r, body, stream)
	if err != nil {
		if errors.Is(err, balancer.ErrNoHealthyBackend) {
			writeJSON(w, http.StatusServiceUnavailable, `{"error":"no healthy backend available"}`)
			return
		}
		writeJSON(w, http.StatusBadGateway, `{"error":"all backends failed"}`)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(responseBytes)

	if !stream && h.cache != nil && prompt != "" {
		respCopy := make([]byte, len(responseBytes))
		copy(respCopy, responseBytes)
		go func(p string, resp []byte) {
			if err := h.cache.Set(context.Background(), p, resp); err != nil {
				h.log.Warn("cache set failed", "err", err)
			} else {
				h.log.Info("cache stored", "prompt_len", len(p))
			}
		}(prompt, respCopy)
	}
}

func (h *Handlers) forwardWithRetry(r *http.Request, body []byte, stream bool) ([]byte, error) {
	tried := map[string]bool{}

	for attempt := 0; attempt < maxRetries; attempt++ {
		backends := h.registry.Healthy()
		var candidates []*registry.Backend
		for _, b := range backends {
			if !tried[b.URL] {
				candidates = append(candidates, b)
			}
		}

		backend, err := h.balancer.Pick(candidates)
		if err != nil {
			return nil, balancer.ErrNoHealthyBackend
		}

		if err := backend.Breaker.Allow(); err != nil {
			h.log.Warn("circuit open, skipping backend",
				"backend", backend.URL,
				"attempt", attempt+1,
			)
			tried[backend.URL] = true
			continue
		}

		buf := &bytes.Buffer{}
		rw := &bufferingResponseWriter{
			ResponseWriter: &noopResponseWriter{header: make(http.Header)},
			buf:            buf,
		}

		forwardErr := h.proxy.Forward(rw, r, backend.URL, "/v1/chat/completions", bytes.NewReader(body))
		if forwardErr != nil {
			backend.Breaker.RecordFailure()
			metrics.BackendFailures.WithLabelValues(backend.URL).Inc()
			h.log.Warn("backend failed, will retry",
				"backend", backend.URL,
				"attempt", attempt+1,
				"err", forwardErr,
				"breaker", backend.Breaker.State(),
			)
			tried[backend.URL] = true
			backoff := time.Duration(attempt+1)*100*time.Millisecond +
				time.Duration(rand.Intn(50))*time.Millisecond
			time.Sleep(backoff)
			continue
		}

		backend.Breaker.RecordSuccess()
		metrics.BackendRequests.WithLabelValues(backend.URL).Inc()
		h.log.Info("request forwarded",
			"backend", backend.URL,
			"attempt", attempt+1,
			"breaker", backend.Breaker.State(),
		)
		return buf.Bytes(), nil
	}

	return nil, errors.New("all retries exhausted")
}

type noopResponseWriter struct {
	header http.Header
}

func (n *noopResponseWriter) Header() http.Header         { return n.header }
func (n *noopResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (n *noopResponseWriter) WriteHeader(int)             {}

type bufferingResponseWriter struct {
	http.ResponseWriter
	buf *bytes.Buffer
}

func (b *bufferingResponseWriter) Write(p []byte) (int, error) {
	b.buf.Write(p)
	return b.ResponseWriter.Write(p)
}

func extractPrompt(body []byte) (prompt string, stream bool) {
	var req struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", false
	}
	stream = req.Stream
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			return req.Messages[i].Content, stream
		}
	}
	return "", stream
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
