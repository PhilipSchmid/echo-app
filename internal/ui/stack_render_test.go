package ui

import (
	"context"
	"crypto/tls"
	"html"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderedStack(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	require.Equal(t, http.StatusOK, response.Code)
	match := regexp.MustCompile(`(?s)<section id="connection-stack"[^>]*>(.*?)</section>`).FindStringSubmatch(response.Body.String())
	require.Len(t, match, 2, "the connection stack must be server-rendered")
	return match[1]
}

func renderedStackLayer(t *testing.T, stack, layer string) string {
	t.Helper()
	match := regexp.MustCompile(`(?s)<article\b[^>]*data-stack-layer="` + layer + `"[^>]*>(.*?)</article>`).FindStringSubmatch(stack)
	require.Len(t, match, 2, "missing layer %s", layer)
	return html.UnescapeString(match[1])
}

func TestStackRendersObservedLayersForEachHTTPTransport(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, listener, remote, peerIP, localIP, peerPort, localPort, transport, alpn string
		major, port                                                                             int
	}{
		{"HTTP1 IPv4", "HTTP/1.1", "HTTP", "192.0.2.8:40501", "192.0.2.8", "192.0.2.9", "40501", "8088", "TCP", "", 1, 8088},
		{"HTTP2 TLS IPv6", "HTTP/2.0", "TLS", "[2001:db8::8]:40502", "2001:db8::8", "2001:db8::9", "40502", "8444", "TCP", "h2", 2, 8444},
		{"HTTP3 QUIC IPv6", "HTTP/3.0", "QUIC", "[2001:db8::8]:40503", "2001:db8::8", "2001:db8::9", "40503", "4433", "QUIC / UDP", "h3", 3, 4433},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ui/?layer=observed", nil)
			req.Host, req.RemoteAddr, req.Proto, req.ProtoMajor = "echo.example", tc.remote, tc.protocol, tc.major
			var local net.Addr = &net.TCPAddr{IP: net.ParseIP(tc.localIP), Port: tc.port}
			if tc.major == 3 {
				local = &net.UDPAddr{IP: net.ParseIP(tc.localIP), Port: tc.port}
			}
			req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, local))
			if tc.alpn != "" {
				req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, CipherSuite: tls.TLS_AES_128_GCM_SHA256, ServerName: "echo.example", NegotiatedProtocol: tc.alpn}
			}
			req.Header.Set("X-Forwarded-For", "198.51.100.99")
			req.Header.Set("Forwarded", "for=198.51.100.99;by=198.51.100.100")
			req.Header.Set("X-Forwarded-Port", "12345")
			response := httptest.NewRecorder()
			NewHandler(&config.Config{PrintHeaders: true}, tc.listener).ServeHTTP(response, req)
			stack := renderedStack(t, response)
			application := renderedStackLayer(t, stack, "application")
			for _, value := range []string{"GET", tc.protocol, "echo.example", "/ui/?layer=observed"} {
				assert.Contains(t, application, value)
			}
			transport := renderedStackLayer(t, stack, "transport")
			for _, value := range []string{tc.transport, tc.peerPort, tc.localPort} {
				assert.Contains(t, transport, value)
			}
			assert.NotContains(t, transport, "12345")
			internet := renderedStackLayer(t, stack, "internet")
			assert.Contains(t, internet, tc.peerIP)
			assert.Contains(t, internet, tc.localIP)
			assert.NotContains(t, internet, "198.51.100.99")
			assert.NotContains(t, internet, "198.51.100.100")
			security := renderedStackLayer(t, stack, "security")
			if tc.alpn != "" {
				for _, value := range []string{"TLS 1.3", "TLS_AES_128_GCM_SHA256", "echo.example", tc.alpn} {
					assert.Contains(t, security, value)
				}
			} else {
				assert.NotContains(t, security, "TLS 1.3")
				assert.NotContains(t, security, "TLS_AES_128_GCM_SHA256")
				assert.Contains(t, security, "This request used HTTP. Visit HTTPS to use TLS.")
			}
			assert.NotContains(t, strings.ToLower(stack), "proxy")
			assert.NotContains(t, strings.ToLower(stack), "wi-fi")
			assert.NotContains(t, stack, `data-stack-layer="link"`)
			assert.NotContains(t, stack, "Network access")
			assert.NotContains(t, stack, "Not exposed")
			assert.NotContains(t, stack, "Client → gateway:")
			assert.NotContains(t, stack, "app-connection")
			assert.Len(t, regexp.MustCompile(`data-stack-layer="`).FindAllString(stack, -1), 4)
			page := response.Body.String()
			visitIndex, stackIndex := strings.Index(page, `<section class="visit-panel"`), strings.Index(page, `<section id="connection-stack"`)
			require.NotEqual(t, -1, visitIndex)
			assert.Less(t, visitIndex, stackIndex)
			assert.NotContains(t, page, `id="request-form"`)
			assert.NotContains(t, page, `id="websocket-form"`)
		})
	}
}

func TestStackHeadersFollowInstanceRedactionAndVisibility(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "headers disabled", true: "headers enabled"}[enabled], func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
			req.Header.Set("X-Stack-Demo", "visible-sample")
			req.Header.Set("Authorization", "Bearer synthetic-stack-secret")
			req.Header.Set("Cookie", "session=synthetic-stack-cookie")
			req.Header.Set("X-Password", "synthetic-stack-password")
			response := httptest.NewRecorder()
			NewHandler(&config.Config{PrintHeaders: enabled}, "HTTP").ServeHTTP(response, req)
			stack := renderedStack(t, response)
			for _, value := range []string{"synthetic-stack-secret", "synthetic-stack-cookie", "synthetic-stack-password"} {
				assert.NotContains(t, stack, value)
			}
			if enabled {
				assert.Contains(t, stack, `<details class="stack-headers" open>`)
				assert.NotContains(t, stack, `tabindex="0"`)
				for _, value := range []string{"X-Stack-Demo", "visible-sample", "Authorization", "Cookie", "X-Password", "[REDACTED]"} {
					assert.Contains(t, stack, value)
				}
			} else {
				assert.NotContains(t, stack, "X-Stack-Demo")
				assert.NotContains(t, stack, "visible-sample")
			}
		})
	}
}

func TestStackEscapesHostileRequestValues(t *testing.T) {
	const payload = `</code><svg onload="alert(1)">`
	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	req.Host = payload
	req.Header.Set("X-Stack-Demo", payload)
	response := httptest.NewRecorder()
	NewHandler(&config.Config{PrintHeaders: true}, "HTTP").ServeHTTP(response, req)
	stack := renderedStack(t, response)
	assert.NotContains(t, stack, payload)
	assert.NotContains(t, stack, `<svg onload=`)
	assert.Contains(t, html.UnescapeString(stack), payload)
}
