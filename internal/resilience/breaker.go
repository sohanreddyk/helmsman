package resilience

import (
	"errors"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when a backend's circuit breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker open")

type state int

const (
	stateClosed   state = iota // normal — requests go through
	stateOpen                  // tripped — requests fast-fail
	stateHalfOpen              // cooldown elapsed — one probe allowed
)

// Breaker is a per-backend circuit breaker.
type Breaker struct {
	mu           sync.Mutex
	state        state
	failures     int
	maxFailures  int
	cooldown     time.Duration
	lastFailure  time.Time
	probeInFlight bool // ensures only one probe goes through in half-open
}

func NewBreaker(maxFailures int, cooldown time.Duration) *Breaker {
	return &Breaker{
		maxFailures: maxFailures,
		cooldown:    cooldown,
	}
}

// Allow returns nil if the request should proceed, ErrCircuitOpen if not.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case stateClosed:
		return nil
	case stateOpen:
		if time.Since(b.lastFailure) >= b.cooldown {
			b.state = stateHalfOpen
			b.probeInFlight = true
			return nil // let the probe through
		}
		return ErrCircuitOpen
	case stateHalfOpen:
		if b.probeInFlight {
			return ErrCircuitOpen // only one probe at a time
		}
		b.probeInFlight = true
		return nil
	}
	return nil
}

// RecordSuccess closes the circuit.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.state = stateClosed
	b.probeInFlight = false
}

// RecordFailure increments the failure count and potentially opens the circuit.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	b.lastFailure = time.Now()
	b.probeInFlight = false
	if b.failures >= b.maxFailures {
		b.state = stateOpen
	}
}

// State returns a string label for logging/metrics.
func (b *Breaker) State() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case stateClosed:
		return "closed"
	case stateOpen:
		return "open"
	case stateHalfOpen:
		return "half-open"
	}
	return "unknown"
}
