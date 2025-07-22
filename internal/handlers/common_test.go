package handlers

import (
	"testing"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestGetHostname(t *testing.T) {
	// First call should cache the hostname
	hostname1 := getHostname()

	// Second call should return the cached value
	hostname2 := getHostname()

	// They should be equal
	assert.Equal(t, hostname1, hostname2)

	// Should not be empty unless there's an error
	if hostname1 != "unknown" {
		assert.NotEmpty(t, hostname1)
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		expected   string
	}{
		{
			name:       "valid address with port",
			remoteAddr: "192.168.1.1:12345",
			expected:   "192.168.1.1",
		},
		{
			name:       "IPv6 address with port",
			remoteAddr: "[2001:db8::1]:12345",
			expected:   "2001:db8::1",
		},
		{
			name:       "address without port",
			remoteAddr: "192.168.1.1",
			expected:   "192.168.1.1",
		},
		{
			name:       "empty address",
			remoteAddr: "",
			expected:   "",
		},
		{
			name:       "malformed address",
			remoteAddr: "not-an-address",
			expected:   "not-an-address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractIP(tt.remoteAddr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewBaseResponse(t *testing.T) {
	cfg := &config.Config{
		Message: "test-message",
		Node:    "test-node",
	}

	tests := []struct {
		name       string
		listener   string
		remoteAddr string
	}{
		{
			name:       "HTTP listener",
			listener:   "HTTP",
			remoteAddr: "192.168.1.1:12345",
		},
		{
			name:       "TCP listener",
			listener:   "TCP",
			remoteAddr: "10.0.0.1:9090",
		},
		{
			name:       "gRPC listener",
			listener:   "gRPC",
			remoteAddr: "[::1]:50051",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := NewBaseResponse(cfg, tt.listener, tt.remoteAddr)

			assert.NotEmpty(t, resp.Timestamp)
			assert.Equal(t, cfg.Message, resp.Message)
			assert.NotEmpty(t, resp.Hostname)
			assert.Equal(t, tt.listener, resp.Listener)
			assert.Equal(t, cfg.Node, resp.Node)
			assert.NotEmpty(t, resp.SourceIP)
		})
	}
}

func TestNewBaseResponse_EmptyConfig(t *testing.T) {
	cfg := &config.Config{}

	resp := NewBaseResponse(cfg, "HTTP", "192.168.1.1:8080")

	assert.NotEmpty(t, resp.Timestamp)
	assert.Empty(t, resp.Message)
	assert.NotEmpty(t, resp.Hostname)
	assert.Equal(t, "HTTP", resp.Listener)
	assert.Empty(t, resp.Node)
	assert.Equal(t, "192.168.1.1", resp.SourceIP)
}

// TestHostnameCaching tests that hostname is properly cached
