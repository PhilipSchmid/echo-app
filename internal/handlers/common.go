package handlers

import (
	"net"
	"os"
	"sync"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/sirupsen/logrus"
)

var (
	hostname     string
	hostnameOnce sync.Once
	hostnameErr  error
)

// getHostname returns the cached hostname
func getHostname() string {
	hostnameOnce.Do(func() {
		hostname, hostnameErr = os.Hostname()
		if hostnameErr != nil {
			logrus.Warnf("Failed to get hostname: %v", hostnameErr)
			hostname = "unknown"
		}
	})
	return hostname
}

// BaseResponse contains common fields for all responses
type BaseResponse struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message,omitempty"`
	Hostname  string `json:"hostname"`
	Listener  string `json:"listener"`
	Node      string `json:"node,omitempty"`
	SourceIP  string `json:"source_ip"`
}

// NewBaseResponse creates a base response with common fields
func NewBaseResponse(cfg *config.Config, listener string, remoteAddr string) BaseResponse {
	sourceIP := extractIP(remoteAddr)

	return BaseResponse{
		Timestamp: time.Now().Format(time.RFC3339),
		Message:   cfg.Message,
		Hostname:  getHostname(),
		Listener:  listener,
		Node:      cfg.Node,
		SourceIP:  sourceIP,
	}
}

// extractIP extracts the IP address from a remote address string
func extractIP(remoteAddr string) string {
	if remoteAddr == "" {
		return ""
	}

	// Try to split host and port
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// If splitting fails, it might be just an IP without port
		// or a malformed address
		logrus.Debugf("Failed to split host:port from %s: %v", remoteAddr, err)
		// Return the original address as fallback
		return remoteAddr
	}

	return ip
}

// normalizeEndpoint normalizes HTTP endpoints to prevent high cardinality in metrics
// Known paths are preserved, all others are grouped as "other"
func normalizeEndpoint(path string) string {
	// List of known paths to track individually
	knownPaths := map[string]bool{
		"/":        true,
		"/health":  true,
		"/ready":   true,
		"/metrics": true,
	}

	if knownPaths[path] {
		return path
	}

	return "other"
}
