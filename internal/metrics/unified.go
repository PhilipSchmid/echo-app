package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestsTotal tracks total requests across all listeners
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "echo_app_requests_total",
			Help: "Total number of requests",
		},
		[]string{"listener", "method", "endpoint"},
	)

	// RequestDuration tracks request duration across all listeners
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "echo_app_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"listener", "method", "endpoint"},
	)

	// ErrorsTotal tracks total errors across all listeners
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "echo_app_errors_total",
			Help: "Total number of errors",
		},
		[]string{"listener", "error_type"},
	)

	// ActiveConnections tracks active connections for connection-based listeners
	ActiveConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "echo_app_active_connections",
			Help: "Number of active connections",
		},
		[]string{"listener"},
	)
)

// RecordRequest records a successful request
func RecordRequest(listener, method, endpoint string, duration float64) {
	RequestsTotal.WithLabelValues(listener, method, endpoint).Inc()
	RequestDuration.WithLabelValues(listener, method, endpoint).Observe(duration)
}

// RecordError records an error
func RecordError(listener, errorType string) {
	ErrorsTotal.WithLabelValues(listener, errorType).Inc()
}

// ConnectionOpened increments active connections
func ConnectionOpened(listener string) {
	ActiveConnections.WithLabelValues(listener).Inc()
}

// ConnectionClosed decrements active connections
func ConnectionClosed(listener string) {
	ActiveConnections.WithLabelValues(listener).Dec()
}
