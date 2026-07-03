package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"

	"github.com/sohanreddy/helmsman/internal/balancer"
	"github.com/sohanreddy/helmsman/internal/proxy"
	"github.com/sohanreddy/helmsman/internal/registry"
)

type Handlers struct {
	registry *registry.Registry
	balancer *balancer.RoundRobin
	proxy    *proxy.Proxy
	log      *slog.Logger
}

func NewHandlers(reg *registry.Registry, bal *balancer.RoundRobin, p *proxy.Proxy, log *slog.Logger) *Handlers {
	return &Handlers{registry: reg, balancer: bal, proxy: p, log: log}
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

	backend, err := h.balancer.Pick(h.registry.Healthy())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, `{"error":"no healthy backend available"}`)
		return
	}

	h.log.Info("routing request", "backend", backend.URL)

	if err := h.proxy.Forward(w, r, backend.URL, "/v1/chat/completions", bytes.NewReader(body)); err != nil {
		h.log.Error("backend forward failed", "backend", backend.URL, "err", err)
		writeJSON(w, http.StatusBadGateway, `{"error":"backend unavailable"}`)
	}
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
