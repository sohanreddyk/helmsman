package cache

import (
	"math"
	"testing"
)

func TestCosineSimilarity_Identical(t *testing.T) {
	v := []float64{1, 2, 3, 4, 5}
	sim := cosineSimilarity(v, v)
	if math.Abs(sim-1.0) > 1e-9 {
		t.Errorf("identical vectors: expected 1.0, got %f", sim)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{-1, 0, 0}
	sim := cosineSimilarity(a, b)
	if math.Abs(sim+1.0) > 1e-9 {
		t.Errorf("opposite vectors: expected -1.0, got %f", sim)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{0, 1, 0}
	sim := cosineSimilarity(a, b)
	if math.Abs(sim) > 1e-9 {
		t.Errorf("orthogonal vectors: expected 0.0, got %f", sim)
	}
}

func TestCosineSimilarity_LengthMismatch(t *testing.T) {
	a := []float64{1, 2, 3}
	b := []float64{1, 2}
	sim := cosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("length mismatch: expected 0, got %f", sim)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float64{0, 0, 0}
	b := []float64{1, 2, 3}
	sim := cosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("zero vector: expected 0, got %f", sim)
	}
}

func TestCosineSimilarity_AboveThreshold(t *testing.T) {
	// Slightly perturbed vector should be close to 1.0
	a := []float64{1.0, 2.0, 3.0}
	b := []float64{1.01, 2.01, 3.01}
	sim := cosineSimilarity(a, b)
	if sim < 0.999 {
		t.Errorf("near-identical vectors: expected > 0.999, got %f", sim)
	}
}
