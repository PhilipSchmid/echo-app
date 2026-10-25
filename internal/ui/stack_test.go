package ui

import (
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	"github.com/stretchr/testify/assert"
)

func TestStackTransportUsesObservedHTTPVersion(t *testing.T) {
	for _, tc := range []struct{ version, transport string }{
		{"HTTP/1.0", "TCP"},
		{"HTTP/1.1", "TCP"},
		{"HTTP/2.0", "TCP"},
		{"HTTP/3", "QUIC / UDP"},
		{"HTTP/3.0", "QUIC / UDP"},
		{"HTTP/9.0", "Not observed"},
		{"", "Not observed"},
	} {
		t.Run(tc.version, func(t *testing.T) {
			request := handlers.HTTPResponse{HTTPVersion: tc.version, Headers: map[string][]string{"X-Forwarded-Proto": {"https"}}}
			assert.Equal(t, tc.transport, newStackDetails(request).Transport)
		})
	}
}

func TestStackSocketsUseObservedAddresses(t *testing.T) {
	for _, tc := range []struct {
		name, remote, local, peerPort, localIP, localPort string
	}{
		{"IPv4", "192.0.2.8:40500", "192.0.2.9:8080", "40500", "192.0.2.9", "8080"},
		{"IPv6", "[2001:db8::8]:40501", "[2001:db8::9]:8443", "40501", "2001:db8::9", "8443"},
		{"mapped IPv4", "[::ffff:192.0.2.8]:40502", "[::ffff:192.0.2.9]:8080", "40502", "192.0.2.9", "8080"},
		{"scoped IPv6", "[fe80::8%en0]:40503", "[fe80::9%en0]:8080", "40503", "fe80::9%en0", "8080"},
		{"empty", "", "", "", "", ""},
		{"malformed", "unknown", "not-an-address", "", "", ""},
		{"hostname", "peer.example:40500", "server.example:8080", "", "", ""},
		{"missing ports", "2001:db8::8", "192.0.2.9", "", "", ""},
		{"invalid ports", "192.0.2.8:65536", "192.0.2.9:abc", "", "", ""},
		{"unassigned ports", "192.0.2.8:0", "[::1]:0", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := handlers.HTTPResponse{
				HTTPVersion: "HTTP/1.1", RemoteAddr: tc.remote, LocalAddr: tc.local,
				Headers: map[string][]string{
					"Forwarded":        {"for=198.51.100.8:5555;by=198.51.100.9:443"},
					"X-Forwarded-For":  {"198.51.100.8"},
					"X-Forwarded-Port": {"443"},
				},
			}
			assert.Equal(t, stackDetails{
				Transport: "TCP", PeerPort: tc.peerPort, LocalIP: tc.localIP, LocalPort: tc.localPort,
			}, newStackDetails(request))
		})
	}
}
