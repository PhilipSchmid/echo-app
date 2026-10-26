package metrics

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestsTotal tracks total requests across all listeners
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "echo_observatory_requests_total",
			Help: "Completed requests, including unsuccessful responses; streaming RPCs count when they end.",
		},
		[]string{"listener", "method", "endpoint"},
	)

	// RequestDuration tracks request duration across all listeners
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "echo_observatory_request_duration_seconds",
			Help:    "Request handling duration in seconds, including unsuccessful responses and full streaming RPC lifetimes.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 300},
		},
		[]string{"listener", "method", "endpoint"},
	)

	// ErrorsTotal tracks total errors across all listeners
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "echo_observatory_errors_total",
			Help: "Total number of errors",
		},
		[]string{"listener", "error_type"},
	)

	// ActiveConnections tracks active connections for connection-based listeners
	ActiveConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "echo_observatory_active_connections",
			Help: "Accepted transport connections currently open; WebSocket counts upgraded sessions separately, so do not sum it with transport connections.",
		},
		[]string{"listener"},
	)
	ConnectionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "echo_observatory_connections_total", Help: "Accepted transport connections; WebSocket counts upgraded sessions separately.",
	}, []string{"listener"})
	ListenerUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "echo_observatory_listener_up", Help: "Whether a configured listener is running (1) or stopped (0).",
	}, []string{"listener"})
	RejectionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "echo_observatory_rejections_total", Help: "Requests or connections rejected by an application admission limit; excludes backpressure.",
	}, []string{"listener", "reason"})
	InFlightRequests = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "echo_observatory_requests_in_flight", Help: "HTTP handlers and gRPC calls currently running; includes WebSocket handler and streaming RPC lifetimes.",
	}, []string{"listener"})
	HTTPResponsesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "echo_observatory_http_responses_total", Help: "HTTP request outcomes by response status, or canceled/panic when no final response was written.",
	}, []string{"listener", "endpoint", "status_code"})
	HTTPResponseBytesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "echo_observatory_http_response_bytes_total", Help: "HTTP response body bytes written; excludes headers and WebSocket frames.",
	}, []string{"listener"})
	TransportBytesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "echo_observatory_transport_bytes_total",
		Help: "Socket bytes received or successfully sent, including TLS/QUIC framing; excludes IP/TCP/UDP headers and traffic outside the app.",
	}, []string{"listener", "direction"})
	HTTPProtocolRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "echo_observatory_http_protocol_requests_total", Help: "Completed HTTP requests by negotiated HTTP protocol and client IP version; uses the socket peer by default or the validated original client with trusted PROXY protocol, never HTTP forwarding headers.",
	}, []string{"listener", "protocol", "ip_version"})
	GRPCResponsesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "echo_observatory_grpc_responses_total", Help: "Completed gRPC calls by method and final gRPC status code.",
	}, []string{"method", "code"})
	WebSocketMessagesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "echo_observatory_websocket_messages_total", Help: "Complete WebSocket messages received or successfully echoed, excluding control frames.",
	}, []string{"direction", "message_type"})
	WebSocketMessageBytesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "echo_observatory_websocket_message_bytes_total", Help: "WebSocket message payload bytes received or successfully echoed; excludes frame overhead.",
	}, []string{"direction", "message_type"})
	ExternalProbeChecksTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "echo_observatory_external_probe_checks_total", Help: "Completed background external readiness checks by result.",
	}, []string{"result"})
	ExternalProbeDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "echo_observatory_external_probe_duration_seconds", Help: "Background external readiness check duration in seconds.", Buckets: prometheus.DefBuckets,
	})
)

// RecordRequest records a completed request using only bounded labels.
func RecordRequest(listener, method, endpoint string, duration float64) {
	listener, method, endpoint = listenerLabel(listener), methodLabel(method), endpointLabel(endpoint)
	RequestsTotal.WithLabelValues(listener, method, endpoint).Inc()
	RequestDuration.WithLabelValues(listener, method, endpoint).Observe(duration)
}

func listenerLabel(listener string) string {
	switch listener {
	case "HTTP", "H2C", "TLS", "QUIC", "TCP", "gRPC", "Metrics", "WebSocket":
		return listener
	default:
		return "other"
	}
}

func methodLabel(method string) string {
	switch method {
	case "GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH",
		"connection", "/echo.EchoService/Echo", "unknown",
		"/grpc.health.v1.Health/Check", "/grpc.health.v1.Health/List", "/grpc.health.v1.Health/Watch",
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo", "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo":
		return method
	default:
		return "other"
	}
}

func endpointLabel(endpoint string) string {
	switch endpoint {
	case "", "/", "/health", "/ready", "/metrics", "/ws", "/robots.txt", "/sitemap.xml",
		"/ui/config.json", "/ui/certificate.json", "ui_assets":
		return endpoint
	case "/ui", "/ui/":
		return "/ui/"
	case "/ui/workbench", "/ui/workbench/":
		return "/ui/workbench/"
	case "/ui/examples", "/ui/examples/":
		return "/ui/examples/"
	case "/ui/connect", "/ui/connect/":
		return "/ui/connect/"
	case "/ui/privacy", "/ui/privacy/":
		return "/ui/privacy/"
	}
	if strings.HasPrefix(endpoint, "/ui/") {
		for _, suffix := range []string{".js", ".css", ".svg", ".woff2"} {
			if strings.HasSuffix(endpoint, suffix) {
				return "ui_assets"
			}
		}
	}
	return "other"
}

// RecordError records an error
func RecordError(listener, errorType string) {
	switch errorType {
	case "panic", "deadline_error", "request_too_large", "read_error", "marshal_error", "write_error", "nil_request",
		"canceled", "read_timeout", "write_timeout", "invalid_utf8", "message_too_large", "response_too_large":
	default:
		errorType = "other"
	}
	ErrorsTotal.WithLabelValues(listenerLabel(listener), errorType).Inc()
}

func RecordRejection(listener, reason string) {
	switch reason {
	case "request_limit", "connection_limit", "session_limit", "request_rate", "peer_rate", "connection_rate",
		"ingress_rate", "egress_rate", "daily_egress", "state_error", "closed", "proxy_peer", "proxy_header":
	default:
		reason = "other"
	}
	RejectionsTotal.WithLabelValues(listenerLabel(listener), reason).Inc()
}

// TransportBytes returns a bounded-label counter. Cache it outside hot read/write paths.
func TransportBytes(listener, direction string) prometheus.Counter {
	if direction != "received" && direction != "sent" {
		direction = "other"
	}
	return TransportBytesTotal.WithLabelValues(listenerLabel(listener), direction)
}

func RecordWebSocketMessage(direction, messageType string, bytes int) {
	if direction != "received" && direction != "sent" {
		return
	}
	if messageType != "text" && messageType != "binary" {
		return
	}
	WebSocketMessagesTotal.WithLabelValues(direction, messageType).Inc()
	WebSocketMessageBytesTotal.WithLabelValues(direction, messageType).Add(float64(max(bytes, 0)))
}

// RecordExternalProbe records an actual background check, never a scrape.
func RecordExternalProbe(success bool, duration float64) {
	result := "failure"
	if success {
		result = "success"
	}
	ExternalProbeChecksTotal.WithLabelValues(result).Inc()
	ExternalProbeDuration.Observe(duration)
}

// ConnectionOpened increments active connections
func ConnectionOpened(listener string) {
	listener = listenerLabel(listener)
	ActiveConnections.WithLabelValues(listener).Inc()
	ConnectionsTotal.WithLabelValues(listener).Inc()
}

// ConnectionClosed decrements active connections
func ConnectionClosed(listener string) {
	ActiveConnections.WithLabelValues(listenerLabel(listener)).Dec()
}
