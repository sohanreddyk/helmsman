package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Addr          string
	BackendURLs   []string
	ProbeInterval time.Duration
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	IdleTimeout   time.Duration
	RedisAddr     string
	RatePerSec    int
	RateBurst     int
	MaxConcurrent int
}

func Load() Config {
	return Config{
		Addr:          getenv("HELMSMAN_ADDR", ":8080"),
		BackendURLs:   parseList("HELMSMAN_BACKEND_URLS", "http://localhost:11434"),
		ProbeInterval: 5 * time.Second,
		ReadTimeout:   15 * time.Second,
		WriteTimeout:  0,
		IdleTimeout:   60 * time.Second,
		RedisAddr:     getenv("HELMSMAN_REDIS_ADDR", "localhost:6379"),
		RatePerSec:    5,
		RateBurst:     10,
		MaxConcurrent: 10,
	}
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func parseList(key, fallback string) []string {
	raw := getenv(key, fallback)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
