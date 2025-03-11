package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

// Prometheus metrics with listener label
var (
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "requests_total",
			Help: "Total number of requests",
		},
		[]string{"listener", "method", "endpoint"},
	)
	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "request_duration_seconds",
			Help:    "Duration of requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"listener", "method", "endpoint"},
	)
	errorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "errors_total",
			Help: "Total number of errors",
		},
		[]string{"listener", "method", "endpoint"},
	)
)

// HTTPResponse defines the structure of the HTTP echo response
type HTTPResponse struct {
	Timestamp    string              `json:"timestamp"`
	Message      string              `json:"message"`
	Hostname     string              `json:"hostname"`
	Listener     string              `json:"listener"`
	Node         string              `json:"node"`
	SourceIP     string              `json:"source_ip"`
	HTTPVersion  string              `json:"http_version,omitempty"`
	HTTPMethod   string              `json:"http_method,omitempty"`
	HTTPEndpoint string              `json:"http_endpoint,omitempty"`
	Headers      map[string][]string `json:"headers,omitempty"`
}

// HTTPHandler returns an HTTP handler function
func HTTPHandler(cfg *config.Config, listener string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		response := buildHTTPResponse(r, cfg, listener)
		data, err := json.Marshal(response)
		if err != nil {
			logrus.Errorf("Failed to marshal JSON: %v", err)
			errorsTotal.WithLabelValues(listener, r.Method, r.URL.Path).Inc()
			w.WriteHeader(http.StatusInternalServerError)
			if _, writeErr := w.Write([]byte("Internal Server Error")); writeErr != nil {
				logrus.Errorf("Failed to write error response: %v", writeErr)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, writeErr := w.Write(data); writeErr != nil {
			logrus.Errorf("Failed to write response: %v", writeErr)
		}
		duration := time.Since(start).Seconds()
		requestsTotal.WithLabelValues(listener, r.Method, r.URL.Path).Inc()
		requestDuration.WithLabelValues(listener, r.Method, r.URL.Path).Observe(duration)
	}
}

// buildHTTPResponse constructs the response struct
func buildHTTPResponse(r *http.Request, cfg *config.Config, listener string) HTTPResponse {
	timestamp := time.Now().Format(time.RFC3339)
	host, _ := os.Hostname()
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)

	response := HTTPResponse{
		Timestamp:    timestamp,
		Message:      cfg.Message,
		Hostname:     host,
		Listener:     listener,
		Node:         cfg.Node,
		SourceIP:     ip,
		HTTPVersion:  r.Proto,
		HTTPMethod:   r.Method,
		HTTPEndpoint: r.URL.Path,
	}
	if cfg.PrintHeaders {
		response.Headers = r.Header
	}
	return response
}
