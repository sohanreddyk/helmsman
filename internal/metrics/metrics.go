package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// All metrics are registered on the default registry via promauto.
var (
	// RequestDuration tracks latency per endpoint and status code.
	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "helmsman_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"path", "status"})

	// RequestsTotal counts all requests by path and status.
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "helmsman_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"path", "status"})

	// CacheHits counts semantic cache hits.
	CacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "helmsman_cache_hits_total",
		Help: "Total number of semantic cache hits.",
	})

	// CacheMisses counts semantic cache misses.
	CacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "helmsman_cache_misses_total",
		Help: "Total number of semantic cache misses.",
	})

	// RateLimitRejections counts requests rejected by the rate limiter.
	RateLimitRejections = promauto.NewCounter(prometheus.CounterOpts{
		Name: "helmsman_rate_limit_rejections_total",
		Help: "Total number of requests rejected by rate limiter.",
	})

	// BackpressureRejections counts requests rejected by the concurrency semaphore.
	BackpressureRejections = promauto.NewCounter(prometheus.CounterOpts{
		Name: "helmsman_backpressure_rejections_total",
		Help: "Total number of requests rejected due to overload.",
	})

	// BackendRequests counts requests forwarded per backend.
	BackendRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "helmsman_backend_requests_total",
		Help: "Total requests forwarded per backend.",
	}, []string{"backend"})

	// BackendFailures counts failed backend requests.
	BackendFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "helmsman_backend_failures_total",
		Help: "Total failed requests per backend.",
	}, []string{"backend"})

	// BackendHealthy tracks current health state per backend (1=healthy, 0=unhealthy).
	BackendHealthy = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "helmsman_backend_healthy",
		Help: "Current health state of each backend (1=healthy, 0=unhealthy).",
	}, []string{"backend"})

	// Inflight tracks currently in-flight requests.
	Inflight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "helmsman_inflight_requests",
		Help: "Number of requests currently being processed.",
	})

	// CircuitBreakerState tracks breaker state per backend (0=closed,1=open,2=half-open).
	CircuitBreakerState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "helmsman_circuit_breaker_state",
		Help: "Circuit breaker state per backend (0=closed, 1=open, 2=half-open).",
	}, []string{"backend"})
)
