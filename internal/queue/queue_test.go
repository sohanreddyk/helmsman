package queue

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestSemaphore_AllowsUpToCapacity(t *testing.T) {
	sem := New(3)
	// Capacity and inflight checks
	if sem.Capacity() != 3 {
		t.Errorf("expected capacity 3, got %d", sem.Capacity())
	}
	if sem.Inflight() != 0 {
		t.Errorf("expected 0 inflight, got %d", sem.Inflight())
	}
}

func TestSemaphore_Returns503WhenFull(t *testing.T) {
	sem := New(1)

	// Block the one slot with a handler that waits
	ready := make(chan struct{})
	unblock := make(chan struct{})

	handler := sem.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(ready)
		<-unblock
	}))

	// First request occupies the slot
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)
	}()

	<-ready // first request is now inflight

	// Second request should be rejected
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	rw2 := httptest.NewRecorder()
	handler.ServeHTTP(rw2, req2)

	if rw2.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rw2.Code)
	}

	close(unblock)
	wg.Wait()
}

func TestSemaphore_HealthEndpointsBypassSemaphore(t *testing.T) {
	sem := New(0) // zero capacity — everything should be rejected except health

	handler := sem.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/healthz", "/readyz"} {
		req := httptest.NewRequest("GET", path, nil)
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)
		if rw.Code != http.StatusOK {
			t.Errorf("path %s: expected 200, got %d", path, rw.Code)
		}
	}
}
