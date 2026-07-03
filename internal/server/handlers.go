package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/sohanreddy/helmsman/internal/balancer"
	"github.com/sohanreddy/helmsman/internal/cache"
	"github.com/sohanreddy/helmsman/internal/proxy"
	"github.com/sohanreddy/helmsman/internal/registry"
)

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

func (h *Handlers) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, `{"error":"failed to read body"}`)
		return
	}

	prompt, stream := extractPrompt(body)

	// Cache lookup (non-streaming only)
	if !stream && h.cache != nil && prompt != "" {
		if cached, hit, err := h.cache.Get(r.Context(), prompt); err == nil && hit {
			h.log.Info("cache hit", "prompt_len", len(prompt))
			w.Header().Set("X-Cache", "HIT")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cached)
			return
		}
	}

	backend, err := h.balancer.Pick(h.registry.Healthy())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, `{"error":"no healthy backend available"}`)
		return
	}

	h.log.Info("routing request", "backend", backend.URL, "stream", stream)

	// Non-streaming: buffer response so we can cache it
	if !stream && h.cache != nil && prompt != "" {
		buf := &bytes.Buffer{}
		rw := &bufferingResponseWriter{ResponseWriter: w, buf: buf}
		if err := h.proxy.Forward(rw, r, backend.URL, "/v1/chat/completions", bytes.NewReader(body)); err != nil {
			h.log.Error("backend forward failed", "backend", backend.URL, "err", err)
			writeJSON(w, http.StatusBadGateway, `{"error":"backend unavailable"}`)
			return
		}
		// Use a fresh context — request context is cancelled after handler returns
		responseBytes := make([]byte, buf.Len())
		copy(responseBytes, buf.Bytes())
		go func(p string, resp []byte) {
			ctx := context.Background()
			if err := h.cache.Set(ctx, p, resp); err != nil {
				h.log.Warn("cache set failed", "err", err)
			} else {
				h.log.Info("cache stored", "prompt_len", len(p))
			}
		}(prompt, responseBytes)
		return
	}

	// Streaming or cache disabled: forward directly
	if err := h.proxy.Forward(w, r, backend.URL, "/v1/chat/completions", bytes.NewReader(body)); err != nil {
		h.log.Error("backend forward failed", "backend", backend.URL, "err", err)
		writeJSON(w, http.StatusBadGateway, `{"error":"backend unavailable"}`)
	}
}

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
