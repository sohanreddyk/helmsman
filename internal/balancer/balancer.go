package balancer

import (
	"errors"
	"sync/atomic"

	"github.com/sohanreddyk/helmsman/internal/registry"
)

var ErrNoHealthyBackend = errors.New("no healthy backend available")

type RoundRobin struct {
	counter atomic.Uint64
}

// Pick selects the next healthy backend using round-robin.
func (rr *RoundRobin) Pick(backends []*registry.Backend) (*registry.Backend, error) {
	if len(backends) == 0 {
		return nil, ErrNoHealthyBackend
	}
	idx := rr.counter.Add(1) - 1
	return backends[idx%uint64(len(backends))], nil
}
