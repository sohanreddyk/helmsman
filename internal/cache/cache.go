package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultTTL       = 24 * time.Hour
	defaultThreshold = 0.95
	embeddingModel   = "nomic-embed-text"
	indexKey         = "cache:index" // Redis Set tracking all cache keys
)

type entry struct {
	Embedding []float64 `json:"embedding"`
	Response  []byte    `json:"response"`
}

// SemanticCache embeds prompts and matches near-duplicates by cosine similarity.
// Keys are tracked in a Redis Set (cache:index) to avoid O(N) KEYS scans.
type SemanticCache struct {
	rdb          *redis.Client
	embedBaseURL string
	threshold    float64
	ttl          time.Duration
	httpClient   *http.Client
}

func New(rdb *redis.Client, embedBaseURL string, threshold float64) *SemanticCache {
	return &SemanticCache{
		rdb:          rdb,
		embedBaseURL: embedBaseURL,
		threshold:    threshold,
		ttl:          defaultTTL,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Get checks for a semantically similar cached response.
func (c *SemanticCache) Get(ctx context.Context, prompt string) ([]byte, bool, error) {
	queryVec, err := c.embed(ctx, prompt)
	if err != nil {
		return nil, false, fmt.Errorf("embed query: %w", err)
	}

	// Use SMEMBERS on the index set — O(N) on the set size, not on all Redis keys
	keys, err := c.rdb.SMembers(ctx, indexKey).Result()
	if err != nil || len(keys) == 0 {
		return nil, false, nil
	}

	var bestSim float64
	var bestResponse []byte

	for _, key := range keys {
		raw, err := c.rdb.Get(ctx, key).Bytes()
		if err != nil {
			// Key expired — clean up the index
			c.rdb.SRem(ctx, indexKey, key)
			continue
		}
		var e entry
		if err := json.Unmarshal(raw, &e); err != nil {
			continue
		}
		sim := cosineSimilarity(queryVec, e.Embedding)
		if sim > bestSim {
			bestSim = sim
			bestResponse = e.Response
		}
	}

	if bestSim >= c.threshold {
		return bestResponse, true, nil
	}
	return nil, false, nil
}

// Set stores a prompt's embedding and its response in Redis.
func (c *SemanticCache) Set(ctx context.Context, prompt string, response []byte) error {
	vec, err := c.embed(ctx, prompt)
	if err != nil {
		return fmt.Errorf("embed for storage: %w", err)
	}

	e := entry{Embedding: vec, Response: response}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("cache:%x", sha256.Sum256([]byte(prompt)))

	pipe := c.rdb.Pipeline()
	pipe.Set(ctx, key, raw, c.ttl)
	pipe.SAdd(ctx, indexKey, key)
	_, err = pipe.Exec(ctx)
	return err
}

func (c *SemanticCache) embed(ctx context.Context, text string) ([]float64, error) {
	payload, _ := json.Marshal(map[string]string{
		"model":  embeddingModel,
		"prompt": text,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.embedBaseURL+"/api/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding returned")
	}
	return result.Embedding, nil
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
