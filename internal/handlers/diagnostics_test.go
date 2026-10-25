package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPDiagnosticsPreservePeerAndOriginalURI(t *testing.T) {
	logs := captureDebugLogs(t)
	req := httptest.NewRequest(http.MethodGet, "/a%2Fb?token=query-secret&item=42", nil)
	req.Host = "routing.example:8443"
	req.RemoteAddr = "[2001:db8::1]:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8443}))
	req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, CipherSuite: tls.TLS_AES_128_GCM_SHA256, ServerName: "tls.example", NegotiatedProtocol: "h2"}
	w := httptest.NewRecorder()
	HTTPHandler(&config.Config{PrintHeaders: true, MaxRequestSize: 4}, "TLS")(w, req)
	var response HTTPResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "routing.example:8443", response.HTTPHost)
	assert.Equal(t, "/a%2Fb?token=query-secret&item=42", response.HTTPRequestURI)
	assert.Equal(t, "/a/b", response.HTTPEndpoint)
	assert.Equal(t, "[2001:db8::1]:1234", response.RemoteAddr)
	assert.Equal(t, "2001:db8::1", response.SourceIP)
	assert.Equal(t, "127.0.0.1:8443", response.LocalAddr)
	assert.Equal(t, []string{"198.51.100.9"}, response.Headers["X-Forwarded-For"])
	assert.Equal(t, "TLS 1.3", response.TLSVersion)
	assert.Equal(t, "TLS_AES_128_GCM_SHA256", response.TLSCipherSuite)
	assert.Equal(t, "tls.example", response.TLSServerName)
	assert.Equal(t, "h2", response.TLSNegotiatedProtocol)
	assert.NotContains(t, logs.String(), "query-secret")
}

func TestHTTPDiagnosticsReflectNegotiatedTLS(t *testing.T) {
	server := httptest.NewUnstartedServer(HTTPHandler(&config.Config{MaxRequestSize: 4}, "TLS"))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := server.Client()
	transport := client.Transport.(*http.Transport)
	transport.ForceAttemptHTTP2 = true
	transport.TLSClientConfig.ServerName = "example.com"
	response, err := client.Get(server.URL + "/original?item=42")
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	var diagnostic HTTPResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&diagnostic))
	assert.Equal(t, "HTTP/2.0", diagnostic.HTTPVersion)
	assert.Equal(t, "h2", diagnostic.TLSNegotiatedProtocol)
	assert.Equal(t, "example.com", diagnostic.TLSServerName)
	assert.Equal(t, tls.VersionName(response.TLS.Version), diagnostic.TLSVersion)
	assert.Equal(t, tls.CipherSuiteName(response.TLS.CipherSuite), diagnostic.TLSCipherSuite)
	assert.Equal(t, server.Listener.Addr().String(), diagnostic.LocalAddr)
	assert.Equal(t, "/original?item=42", diagnostic.HTTPRequestURI)
}

func TestHTTPRequestURIFallback(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.com/a%2Fb?item=42", nil)
	require.NoError(t, err)
	assert.Equal(t, "/a%2Fb?item=42", buildHTTPResponse(req, &config.Config{}, "HTTP").HTTPRequestURI)
}
