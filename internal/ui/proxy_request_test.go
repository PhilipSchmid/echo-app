package ui

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/connection"
	"github.com/stretchr/testify/assert"
)

type proxyRequestAddr struct {
	net.Addr
	socket net.Addr
}

func (a proxyRequestAddr) SocketAddr() net.Addr { return a.socket }

type proxyRequestConn struct {
	net.Conn
	remote net.Addr
}

func (c proxyRequestConn) RemoteAddr() net.Addr { return c.remote }

func TestPageShowsTrustedClientFamilyAndPreservesSocketHop(t *testing.T) {
	remote := proxyRequestAddr{
		Addr:   &net.TCPAddr{IP: net.ParseIP("2001:db8::42"), Port: 40500},
		socket: &net.TCPAddr{IP: net.ParseIP("172.16.0.2"), Port: 50000},
	}
	local := proxyRequestAddr{
		Addr:   &net.TCPAddr{IP: net.ParseIP("2001:db8::43"), Port: 443},
		socket: &net.TCPAddr{IP: net.ParseIP("172.16.0.3"), Port: 8443},
	}
	req := httptest.NewRequest(http.MethodGet, "https://echo.example/ui/", nil)
	req.RemoteAddr = remote.String()
	req.Proto, req.ProtoMajor = "HTTP/2.0", 2
	req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13}
	req.Header.Set("X-Forwarded-For", "198.51.100.99")
	ctx := connection.WithConn(req.Context(), proxyRequestConn{remote: remote})
	ctx = context.WithValue(ctx, http.LocalAddrContextKey, local)
	response := httptest.NewRecorder()
	NewHandler(&config.Config{}, "TLS").ServeHTTP(response, req.WithContext(ctx))
	visit := visitRequest(t, response)
	assert.Equal(t, "2001:db8::42", visit.SourceIP)
	assert.Equal(t, "IPv6", visit.IPVersion)
	assert.Equal(t, "172.16.0.2", visit.SocketPeerIP)
	assert.Equal(t, "IPv4", visit.SocketPeerIPVersion)
	assert.Contains(t, response.Body.String(), `<dt>Client IP</dt>`)
	assert.Contains(t, response.Body.String(), `Original client IP family from a trusted PROXY protocol peer">IPv6</span>`)
	stack := renderedStack(t, response)
	transport := renderedStackLayer(t, stack, "transport")
	assert.Contains(t, transport, `<dt>Client port</dt><dd>40500</dd>`)
	assert.Contains(t, transport, `<dt>Destination port</dt><dd>443</dd>`)
	assert.Contains(t, transport, `<dt>Socket peer port</dt><dd>50000</dd>`)
	assert.Contains(t, transport, `<dt>App socket port</dt><dd>8443</dd>`)
	internet := renderedStackLayer(t, stack, "internet")
	assert.Contains(t, internet, `<dt>Client IP</dt><dd>2001:db8::42</dd>`)
	assert.Contains(t, internet, `<dt>Destination IP</dt><dd>2001:db8::43</dd>`)
	assert.Contains(t, internet, `<dt>Socket peer IP</dt><dd>172.16.0.2`)
	assert.Contains(t, internet, `<dt>App socket IP</dt><dd>172.16.0.3`)
	assert.NotContains(t, internet, "198.51.100.99")
}

func TestProxyConfigurationAloneDoesNotAssertAClientIdentityOrExposeTrustPolicy(t *testing.T) {
	cfg := &config.Config{ProxyProtocol: true, ProxyTrustedCIDRs: []string{"10.42.0.0/16"}}
	handler := NewHandler(cfg, "HTTP")
	for _, path := range []string{"/ui/", "/ui/config.json"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "192.0.2.8:40500"
		req.Header.Set("X-Forwarded-For", "2001:db8::99")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		assert.NotContains(t, response.Body.String(), "10.42.0.0/16")
		if path == "/ui/" {
			assert.Equal(t, "192.0.2.8", visitRequest(t, response).SourceIP)
			assert.Contains(t, response.Body.String(), `<dt>Socket peer IP</dt>`)
			assert.NotContains(t, response.Body.String(), `<dt>Client IP</dt>`)
		}
	}
}
