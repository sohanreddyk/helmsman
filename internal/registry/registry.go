package registry

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type Backend struct {
	URL     string
	Healthy bool
}

type Registry struct {
	mu       sync.RWMutex
	backends []*Backend
	log      *slog.Logger
	client   *http.Client
}

func New(urls []string, log *slog.Logger) *Registry {
	backends := make([]*Backend, len(urls))
	for i, u := range urls {
		backends[i] = &Backend{URL: u, Healthy: true} // optimistic start
	}
	return &Registry{
		backends: backends,
		log:      log,
		client:   &http.Client{Timeout: 3 * time.Second},
	}
}

// Healthy returns all currently healthy backends (snapshot).
func (r *Registry) Healthy() []*Backend {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Backend, 0, len(r.backends))
	for _, b := range r.backends {
		if b.Healthy {
			out = append(out, b)
		}
	}
	return out
}

// All returns every backend regardless of health (for the stats endpoint later).
func (r *Registry) All() []*Backend {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Backend, len(r.backends))
	copy(out, r.backends)
	return out
}

// StartProbes launches a background goroutine that probes each backend every
// interval. It stops when ctx is cancelled.
func (r *Registry) StartProbes(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.probeAll()
			}
		}
	}()
}

func (r *Registry) probeAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, b := range r.backends {
		resp, err := r.client.Get(b.URL + "/")
		healthy := err == nil && resp.StatusCode < 500
		if resp != nil {
			resp.Body.Close()
		}
		if b.Healthy != healthy {
			r.log.Info("backend health changed",
				"url", b.URL,
				"healthy", healthy,
			)
		}
		b.Healthy = healthy
	}
}
