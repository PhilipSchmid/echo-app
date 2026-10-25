package handlers

import (
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/connection"
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
	Timestamp           string `json:"timestamp"`
	Message             string `json:"message,omitempty"`
	Hostname            string `json:"hostname"`
	Listener            string `json:"listener"`
	Node                string `json:"node,omitempty"`
	SourceIP            string `json:"source_ip"`
	IPVersion           string `json:"ip_version,omitempty"`
	SocketPeerIP        string `json:"socket_peer_ip,omitempty"`
	SocketPeerIPVersion string `json:"socket_peer_ip_version,omitempty"`
	SocketPeerAddr      string `json:"socket_peer_addr,omitempty"`
}

func setSocketPeer(response *BaseResponse, remote net.Addr) {
	socket := connection.SocketAddr(remote)
	if socket == nil {
		return
	}
	response.SocketPeerAddr = socket.String()
	response.SocketPeerIP = extractIP(response.SocketPeerAddr)
	response.SocketPeerIPVersion = ipVersion(response.SocketPeerIP)
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
		IPVersion: ipVersion(sourceIP),
	}
}

func ipVersion(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	// Dual-stack sockets may represent IPv4 peers as ::ffff:a.b.c.d.
	if addr.Unmap().Is4() {
		return "IPv4"
	}
	return "IPv6"
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
		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			logrus.Debugf("Failed to split host:port from %s", remoteAddr)
		}
		// Return the original address as fallback
		return remoteAddr
	}

	return ip
}
