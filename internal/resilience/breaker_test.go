package resilience

import (
	"testing"
	"time"
)

func TestBreaker_InitiallyClosed(t *testing.T) {
	b := NewBreaker(3, 30*time.Second)
	if err := b.Allow(); err != nil {
		t.Errorf("new breaker should be closed, got: %v", err)
	}
	if b.State() != "closed" {
		t.Errorf("expected closed, got %s", b.State())
	}
}

func TestBreaker_OpensAfterMaxFailures(t *testing.T) {
	b := NewBreaker(3, 30*time.Second)
	b.RecordFailure()
	b.RecordFailure()
	if b.State() != "closed" {
		t.Error("should still be closed after 2 failures")
	}
	b.RecordFailure()
	if b.State() != "open" {
		t.Errorf("expected open after 3 failures, got %s", b.State())
	}
}

func TestBreaker_RejectsWhenOpen(t *testing.T) {
	b := NewBreaker(1, 30*time.Second)
	b.RecordFailure() // trips open
	if err := b.Allow(); err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestBreaker_HalfOpenAfterCooldown(t *testing.T) {
	b := NewBreaker(1, 10*time.Millisecond)
	b.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	if err := b.Allow(); err != nil {
		t.Errorf("should allow probe after cooldown, got: %v", err)
	}
	if b.State() != "half-open" {
		t.Errorf("expected half-open, got %s", b.State())
	}
}

func TestBreaker_ClosesOnSuccessFromHalfOpen(t *testing.T) {
	b := NewBreaker(1, 10*time.Millisecond)
	b.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	_ = b.Allow() // probe
	b.RecordSuccess()
	if b.State() != "closed" {
		t.Errorf("expected closed after successful probe, got %s", b.State())
	}
}

func TestBreaker_ReopensOnFailureFromHalfOpen(t *testing.T) {
	b := NewBreaker(1, 10*time.Millisecond)
	b.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	_ = b.Allow() // probe
	b.RecordFailure()
	if b.State() != "open" {
		t.Errorf("expected open after failed probe, got %s", b.State())
	}
}

func TestBreaker_SuccessResetsClosed(t *testing.T) {
	b := NewBreaker(3, 30*time.Second)
	b.RecordFailure()
	b.RecordFailure()
	b.RecordSuccess() // should reset failure count
	b.RecordFailure()
	b.RecordFailure()
	if b.State() != "closed" {
		t.Errorf("success should reset failure count, got %s", b.State())
	}
}
