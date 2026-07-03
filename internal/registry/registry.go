package registry

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/sohanreddyk/helmsman/internal/resilience"
)

type Backend struct {
	URL     string
	Healthy bool
	Breaker *resilience.Breaker
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
		backends[i] = &Backend{
			URL:     u,
			Healthy: true,
			Breaker: resilience.NewBreaker(3, 30*time.Second),
		}
	}
	return &Registry{
		backends: backends,
		log:      log,
		client:   &http.Client{Timeout: 3 * time.Second},
	}
}

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

func (r *Registry) All() []*Backend {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Backend, len(r.backends))
	copy(out, r.backends)
	return out
}

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

// probeAll checks each backend outside the lock, then locks only to update state.
// Holding a write lock during HTTP calls would block all request routing for up
// to the probe timeout — a significant latency spike under load.
func (r *Registry) probeAll() {
	// Snapshot URLs without holding the lock
	r.mu.RLock()
	backends := make([]*Backend, len(r.backends))
	copy(backends, r.backends)
	r.mu.RUnlock()

	type result struct {
		backend *Backend
		healthy bool
	}
	results := make([]result, len(backends))

	for i, b := range backends {
		resp, err := r.client.Get(b.URL + "/")
		healthy := err == nil && resp.StatusCode < 500
		if resp != nil {
			resp.Body.Close()
		}
		results[i] = result{backend: b, healthy: healthy}
	}

	// Lock only to write results
	r.mu.Lock()
	for _, res := range results {
		if res.backend.Healthy != res.healthy {
			r.log.Info("backend health changed",
				"url", res.backend.URL,
				"healthy", res.healthy,
				"breaker", res.backend.Breaker.State(),
			)
		}
		res.backend.Healthy = res.healthy
	}
	r.mu.Unlock()
}
