package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/sohanreddyk/helmsman/internal/balancer"
	"github.com/sohanreddyk/helmsman/internal/cache"
	"github.com/sohanreddyk/helmsman/internal/config"
	"github.com/sohanreddyk/helmsman/internal/proxy"
	"github.com/sohanreddyk/helmsman/internal/queue"
	"github.com/sohanreddyk/helmsman/internal/ratelimit"
	"github.com/sohanreddyk/helmsman/internal/registry"
)

type Server struct {
	http     *http.Server
	registry *registry.Registry
	log      *slog.Logger
}

func New(cfg config.Config, log *slog.Logger) (*Server, error) {
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis unavailable at %s: %w", cfg.RedisAddr, err)
	}
	log.Info("redis connected", "addr", cfg.RedisAddr)

	limiter := ratelimit.New(rdb, cfg.RatePerSec, cfg.RateBurst)
	sem := queue.New(cfg.MaxConcurrent)
	reg := registry.New(cfg.BackendURLs, log)
	bal := &balancer.RoundRobin{}

	var sc *cache.SemanticCache
	if cfg.CacheEnabled {
		sc = cache.New(rdb, cfg.EmbedBaseURL, cfg.CacheThreshold)
		log.Info("semantic cache enabled", "threshold", cfg.CacheThreshold)
	}

	h := NewHandlers(reg, bal, proxy.New(), sc, log)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.HandleFunc("GET /readyz", h.Readyz)
	mux.HandleFunc("GET /stats", h.Stats)
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("POST /v1/chat/completions", h.ChatCompletions)

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
