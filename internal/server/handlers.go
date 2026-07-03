package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"

	"github.com/sohanreddy/helmsman/internal/proxy"
)

type Handlers struct {
	backendURL string
	proxy      *proxy.Proxy
	log        *slog.Logger
}

func NewHandlers(backendURL string, p *proxy.Proxy, log *slog.Logger) *Handlers {
	return &Handlers{backendURL: backendURL, proxy: p, log: log}
}

func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, `{"status":"ok"}`)
}

func (h *Handlers) Readyz(w http.ResponseWriter, r *http.Request) {
	// Phase 2 replaces this with a real backend-health check.
	writeJSON(w, http.StatusOK, `{"status":"ready"}`)
}

func (h *Handlers) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB cap
	if err != nil {
		writeJSON(w, http.StatusBadRequest, `{"error":"failed to read body"}`)
		return
	}
	if err := h.proxy.Forward(w, r, h.backendURL, "/v1/chat/completions", bytes.NewReader(body)); err != nil {
		h.log.Error("backend forward failed", "err", err)
		// Headers may already be written for a stream; only safe to set status if not.
		writeJSON(w, http.StatusBadGateway, `{"error":"backend unavailable"}`)
	}
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
