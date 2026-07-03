package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/sohanreddy/helmsman/internal/config"
	"github.com/sohanreddy/helmsman/internal/proxy"
)

type Server struct {
	http *http.Server
	log  *slog.Logger
}

func New(cfg config.Config, log *slog.Logger) *Server {
	h := NewHandlers(cfg.BackendURL, proxy.New(), log)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.HandleFunc("GET /readyz", h.Readyz)
	mux.HandleFunc("POST /v1/chat/completions", h.ChatCompletions)

	handler := chain(mux,
		RecoverMiddleware(log),
		RequestIDMiddleware,
		LoggingMiddleware(log),
	)

	return &Server{
		http: &http.Server{
			Addr:         cfg.Addr,
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
		log: log,
	}
}

func (s *Server) Start() error {
	s.log.Info("gateway listening", "addr", s.http.Addr)
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info("shutting down")
	return s.http.Shutdown(ctx)
}
