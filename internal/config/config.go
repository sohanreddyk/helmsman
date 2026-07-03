package config

import (
	"os"
	"time"
)

type Config struct {
	Addr         string
	BackendURL   string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func Load() Config {
	return Config{
		Addr:         getenv("HELMSMAN_ADDR", ":8080"),
		BackendURL:   getenv("HELMSMAN_BACKEND_URL", "http://localhost:11434"),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // 0 = no write timeout, required for SSE streaming
		IdleTimeout:  60 * time.Second,
	}
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
