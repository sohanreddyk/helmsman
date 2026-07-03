package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/sohanreddy/helmsman/internal/ratelimit"
)

type ctxKey string

const (
	requestIDKey ctxKey = "request_id"
	apiKeyCtxKey ctxKey = "api_key"
)

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func LoggingMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			id, _ := r.Context().Value(requestIDKey).(string)
			key, _ := r.Context().Value(apiKeyCtxKey).(string)
			log.Info("request",
				"id", id,
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"dur_ms", time.Since(start).Milliseconds(),
				"api_key", key,
			)
		})
	}
}

func RecoverMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error("panic recovered", "err", err)
					http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// AuthMiddleware extracts the Bearer token from the Authorization header.
// For now any non-empty token is accepted; Phase 5 will validate against Postgres.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health endpoints
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		key, found := strings.CutPrefix(auth, "Bearer ")
		if !found || key == "" {
			writeJSON(w, http.StatusUnauthorized, `{"error":"missing or invalid Authorization header"}`)
			return
		}
		ctx := context.WithValue(r.Context(), apiKeyCtxKey, key)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RateLimitMiddleware enforces per-key token bucket limits via Redis.
func RateLimitMiddleware(limiter *ratelimit.Limiter, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, ok := r.Context().Value(apiKeyCtxKey).(string)
			if !ok || key == "" {
				next.ServeHTTP(w, r)
				return
			}
			allowed, err := limiter.Allow(r.Context(), key)
			if err != nil {
				log.Error("rate limiter error", "err", err)
				// Fail open: if Redis is down, let the request through
				next.ServeHTTP(w, r)
				return
			}
			if !allowed {
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusTooManyRequests, `{"error":"rate limit exceeded"}`)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
