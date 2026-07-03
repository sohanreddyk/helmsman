package queue

import (
	"net/http"
)

// Semaphore is a bounded concurrency limiter backed by a buffered channel.
// It caps the number of requests processed simultaneously. When full, new
// requests are rejected immediately with 503 — this is intentional backpressure:
// we shed load fast rather than queue forever and blow up p99 latency.
type Semaphore struct {
	slots chan struct{}
}

func New(maxConcurrent int) *Semaphore {
	return &Semaphore{
		slots: make(chan struct{}, maxConcurrent),
	}
}

// Middleware returns an http.Handler middleware that enforces the concurrency cap.
func (s *Semaphore) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip health endpoints — they must always respond
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}

		// Try to acquire a slot (non-blocking)
		select {
		case s.slots <- struct{}{}:
			// Got a slot — release it when done
			defer func() { <-s.slots }()
			next.ServeHTTP(w, r)
		default:
			// No slots available — shed load immediately
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"server overloaded, try again shortly"}`))
		}
	})
}

// Inflight returns the current number of in-flight requests.
func (s *Semaphore) Inflight() int {
	return len(s.slots)
}

// Capacity returns the max concurrency limit.
func (s *Semaphore) Capacity() int {
	return cap(s.slots)
}
