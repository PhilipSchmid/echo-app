package handlers

import (
	"encoding/json"
	"net"
	"os"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

// Prometheus metrics for TCP
var (
	tcpConnectionsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "tcp_connections_total",
			Help: "Total number of TCP connections",
		},
	)
	tcpConnectionDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "tcp_connection_duration_seconds",
			Help:    "Duration of TCP connections in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
	tcpErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "tcp_errors_total",
			Help: "Total number of TCP errors",
		},
	)
)

// TCPResponse represents the expected structure of the TCP response
type TCPResponse struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Hostname  string `json:"hostname"`
	Listener  string `json:"listener"`
	Node      string `json:"node"`
	SourceIP  string `json:"source_ip"`
}

func TCPHandler(conn net.Conn, cfg *config.Config) {
	start := time.Now()
	defer conn.Close()
	response := buildTCPResponse(conn, cfg)
	data, err := json.Marshal(response)
	if err != nil {
		logrus.Errorf("Failed to marshal JSON: %v", err)
		tcpErrorsTotal.Inc()
		return
	}
	if _, err := conn.Write(data); err != nil {
		logrus.Errorf("Failed to write to connection: %v", err)
		tcpErrorsTotal.Inc()
	}
	duration := time.Since(start).Seconds()
	tcpConnectionsTotal.Inc()
	tcpConnectionDuration.Observe(duration)
}

// buildTCPResponse constructs the response for TCP
func buildTCPResponse(conn net.Conn, cfg *config.Config) TCPResponse {
	timestamp := time.Now().Format(time.RFC3339)
	host, _ := os.Hostname()
	ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())

	return TCPResponse{
		Timestamp: timestamp,
		Message:   cfg.Message,
		Hostname:  host,
		Listener:  "TCP",
		Node:      cfg.Node,
		SourceIP:  ip,
	}
}
