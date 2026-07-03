package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sohanreddy/helmsman/internal/balancer"
	"github.com/sohanreddy/helmsman/internal/config"
	"github.com/sohanreddy/helmsman/internal/proxy"
	"github.com/sohanreddy/helmsman/internal/queue"
	"github.com/sohanreddy/helmsman/internal/ratelimit"
	"github.com/sohanreddy/helmsman/internal/registry"
)

type Server struct {
	http     *http.Server
	registry *registry.Registry
	log      *slog.Logger
}

func New(cfg config.Config, log *slog.Logger) (*Server, error) {
	// Connect to Redis and verify it's reachable
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis unavailable at %s: %w", cfg.RedisAddr, err)
	}
	log.Info("redis connected", "addr", cfg.RedisAddr)

	limiter := ratelimit.New(rdb, cfg.RatePerSec, cfg.RateBurst)
	sem := queue.New(cfg.MaxConcurrent)
	reg := registry.New(cfg.BackendURLs, log)
	bal := &balancer.RoundRobin{}
	h := NewHandlers(reg, bal, proxy.New(), log)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.HandleFunc("GET /readyz", h.Readyz)
	mux.HandleFunc("POST /v1/chat/completions", h.ChatCompletions)

	// Middleware order (outermost → innermost):
	// recover → requestID → auth → ratelimit → queue → logging → handler
	// Queue sits inside rate limit: a rejected request doesn't consume a slot.
	handler := chain(mux,
		RecoverMiddleware(log),
		RequestIDMiddleware,
		AuthMiddleware,
		RateLimitMiddleware(limiter, log),
		sem.Middleware,
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
		registry: reg,
		log:      log,
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.registry.StartProbes(ctx, 5*time.Second)
	s.log.Info("gateway listening", "addr", s.http.Addr)
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info("shutting down")
	return s.http.Shutdown(ctx)
}
