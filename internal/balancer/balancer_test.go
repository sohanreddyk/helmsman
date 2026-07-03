package balancer

import (
	"testing"

	"github.com/sohanreddy/helmsman/internal/registry"
	"github.com/sohanreddy/helmsman/internal/resilience"
	"time"
)

func makeBackend(url string) *registry.Backend {
	return &registry.Backend{
		URL:     url,
		Healthy: true,
		Breaker: resilience.NewBreaker(3, 30*time.Second),
	}
}

func TestRoundRobin_DistributesEvenly(t *testing.T) {
	rr := &RoundRobin{}
	backends := []*registry.Backend{
		makeBackend("http://a"),
		makeBackend("http://b"),
		makeBackend("http://c"),
	}

	counts := map[string]int{}
	for i := 0; i < 9; i++ {
		b, err := rr.Pick(backends)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[b.URL]++
	}

	for _, b := range backends {
		if counts[b.URL] != 3 {
			t.Errorf("expected 3 picks for %s, got %d", b.URL, counts[b.URL])
		}
	}
}

func TestRoundRobin_EmptyReturnsError(t *testing.T) {
	rr := &RoundRobin{}
	_, err := rr.Pick(nil)
	if err != ErrNoHealthyBackend {
		t.Errorf("expected ErrNoHealthyBackend, got %v", err)
	}
}

func TestRoundRobin_SingleBackend(t *testing.T) {
	rr := &RoundRobin{}
	backends := []*registry.Backend{makeBackend("http://only")}
	for i := 0; i < 5; i++ {
		b, err := rr.Pick(backends)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.URL != "http://only" {
			t.Errorf("expected http://only, got %s", b.URL)
		}
	}
}
