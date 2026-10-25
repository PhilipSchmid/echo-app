package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetsAndSecurityHeaders(t *testing.T) {
	handler := NewHandler(&config.Config{}, "HTTP")
	for _, test := range []struct{ path, contentType string }{
		{"/ui/", "text/html; charset=utf-8"},
		{"/ui/workbench/", "text/html; charset=utf-8"},
		{"/ui/examples/", "text/html; charset=utf-8"},
		{"/ui/connect/", "text/html; charset=utf-8"},
		{"/ui/privacy/", "text/html; charset=utf-8"},
		{"/ui/app.css", "text/css; charset=utf-8"},
		{"/ui/app.js", "text/javascript; charset=utf-8"},
		{"/ui/visit.js", "text/javascript; charset=utf-8"},
		{"/ui/certificate.js", "text/javascript; charset=utf-8"},
		{"/ui/common.js", "text/javascript; charset=utf-8"},
		{"/ui/headers.js", "text/javascript; charset=utf-8"},
		{"/ui/presets.js", "text/javascript; charset=utf-8"},
		{"/ui/guide.js", "text/javascript; charset=utf-8"},
		{"/ui/controls.js", "text/javascript; charset=utf-8"},
		{"/ui/highlight.js", "text/javascript; charset=utf-8"},
		{"/ui/terminal.js", "text/javascript; charset=utf-8"},
		{"/ui/privacy.js", "text/javascript; charset=utf-8"},
		{"/ui/logo.svg", "image/svg+xml"},
		{"/ui/favicon.svg", "image/svg+xml"},
		{"/ui/nunito-latin.woff2", "font/woff2"},
		{"/ui/sometype-mono-latin.woff2", "font/woff2"},
		{"/ui/config.json", "application/json"},
		{"/ui/certificate.json", "application/json"},
	} {
		t.Run(test.path, func(t *testing.T) {
			get := httptest.NewRecorder()
			handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, test.path, nil))
			require.Equal(t, http.StatusOK, get.Code)
			assert.NotEmpty(t, get.Body.Bytes())
			assert.Equal(t, test.contentType, get.Header().Get("Content-Type"))
			if test.contentType == "font/woff2" {
				assert.True(t, bytes.HasPrefix(get.Body.Bytes(), []byte("wOF2")), "response must contain a WOFF2 font")
			}
			assert.Equal(t, strconv.Itoa(get.Body.Len()), get.Header().Get("Content-Length"))
			cacheControl := staticCacheControl
			if strings.HasPrefix(test.contentType, "text/html") || test.contentType == "application/json" {
				cacheControl = "no-store"
				assert.Empty(t, get.Header().Get("ETag"))
			}
			assert.Equal(t, cacheControl, get.Header().Get("Cache-Control"))
			assert.Equal(t, "nosniff", get.Header().Get("X-Content-Type-Options"))
			assert.Equal(t, "no-referrer", get.Header().Get("Referrer-Policy"))
			assert.Equal(t, "DENY", get.Header().Get("X-Frame-Options"))
			policy := get.Header().Get("Content-Security-Policy")
			for _, directive := range []string{"default-src 'none'", "script-src 'self'", "style-src 'self'", "connect-src 'self'", "img-src 'self'", "font-src 'self'", "frame-ancestors 'none'", "base-uri 'none'"} {
				assert.Contains(t, policy, directive)
			}
			assert.NotContains(t, policy, "unsafe-inline")
			assert.NotContains(t, policy, "unsafe-eval")
			head := httptest.NewRecorder()
			handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, test.path, nil))
			assert.Equal(t, http.StatusOK, head.Code)
			assert.Empty(t, head.Body.Bytes())
			assert.Equal(t, get.Header(), head.Header())
		})
	}
}

func TestOnlyKnownPathsAndReadMethods(t *testing.T) {
	handler := NewHandler(&config.Config{}, "HTTP")
	for _, path := range []string{"/ui/missing", "/ui/assets/", "/ui/index.html", "/ui/../config.go", "/ui/%2e%2e/config.go", "/ui/app.js/extra", "/ui//app.js", "/ui/.git/config", "/ui/config.json/", "/ui/certificate.json/", "/ui/certificate.json/extra", "/ui/certificate.js/extra", "/ui/certificate.js.map", "/ui/certificate.go", "/ui/workbench.html", "/ui/workbench/missing", "/ui/headers.js/extra", "/ui/headers.js/", "/ui/visit.js/extra", "/ui/visit.js.map", "/ui/examples.html", "/ui/connect.html", "/ui/examples/missing", "/ui/connect/missing", "/ui/privacy.html", "/ui/privacy/missing", "/ui/privacy.js/extra", "/ui/guide.js/extra", "/ui/common.js.map", "/ui/presets.js/", "/ui/controls.js/extra", "/ui/highlight.js/extra", "/ui/terminal.js/extra", "/ui/logo.svg/", "/ui/nunito-latin.woff2/extra", "/ui/sometype-mono-latin.woff2/extra", "/ui/Nunito-OFL.txt", "/ui/SometypeMono-OFL.txt"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			assert.Equal(t, http.StatusNotFound, response.Code)
		})
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, "CUSTOM"} {
		for _, path := range []string{"/ui", "/ui/", "/ui/workbench", "/ui/workbench/", "/ui/examples", "/ui/examples/", "/ui/connect", "/ui/connect/", "/ui/privacy", "/ui/privacy/", "/ui/privacy.js", "/ui/app.css", "/ui/app.js", "/ui/headers.js", "/ui/visit.js", "/ui/common.js", "/ui/presets.js", "/ui/guide.js", "/ui/controls.js", "/ui/highlight.js", "/ui/terminal.js", "/ui/logo.svg", "/ui/favicon.svg", "/ui/nunito-latin.woff2", "/ui/sometype-mono-latin.woff2", "/ui/config.json", "/ui/certificate.json"} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(method, path, strings.NewReader(`{"ui":false}`)))
			assert.Equal(t, http.StatusMethodNotAllowed, response.Code)
			assert.Equal(t, "GET, HEAD", response.Header().Get("Allow"))
		}
	}
}

func TestConfigExposesOnlyPublicAllowlist(t *testing.T) {
	cfg := &config.Config{
		H2C: true, TLS: true, TCP: true, GRPC: true, GRPCTLS: true, QUIC: true, Metrics: true,
		HTTPPort: "8081", TLSPort: "8444", TCPPort: "9091", GRPCPort: "50052", QUICPort: "4434", MetricsPort: "3001",
		PrintHeaders: true, ResponseControls: true, MaxRequestSize: 1234, MaxResponseSize: 4321,
		Message: "secret-message", Node: "private-node", UnsafeHeaders: true,
		TLSCertFile: "private-certificate.pem", TLSKeyFile: "private-key.pem", TLSClientCAFile: "private-ca.pem",
		ExternalReadinessProbe: config.ExternalReadinessProbe{Target: "https://user:secret@internal"},
	}
	response := httptest.NewRecorder()
	NewHandler(cfg, "HTTP").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/config.json", nil))
	var got map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &got))
	assert.Equal(t, map[string]any{
		"h2c": true, "tls": true, "tcp": true, "grpc": true, "grpc_tls": true, "quic": true,
		"http_port": "8081", "tls_port": "8444", "tcp_port": "9091", "grpc_port": "50052", "quic_port": "4434",
		"print_headers": true, "unsafe_headers": true, "response_controls": true, "demo_certificate": false, "max_request_size": float64(1234), "max_response_size": float64(4321),
	}, got)
	for _, secret := range []string{cfg.Message, cfg.Node, cfg.TLSCertFile, cfg.TLSKeyFile, cfg.TLSClientCAFile, cfg.ExternalReadinessProbe.Target} {
		assert.NotContains(t, response.Body.String(), secret)
	}
	response = httptest.NewRecorder()
	NewHandler(&config.Config{}, "HTTP").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/config.json", nil))
	assert.Contains(t, response.Body.String(), `"demo_certificate":true`)
	response = httptest.NewRecorder()
	NewHandler(&config.Config{ACMEDomain: "private.example.com", ACMECacheDir: "/private/acme", ACMEAcceptTOS: true, ACMEStaging: true}, "HTTP").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/config.json", nil))
	assert.Contains(t, response.Body.String(), `"demo_certificate":false`)
	assert.NotContains(t, response.Body.String(), "private.example.com")
	assert.NotContains(t, response.Body.String(), "/private/acme")
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &got))
	assert.Len(t, got, 17, "ACME settings must not expand the public config allowlist")
}

func TestConcurrentConfigRequests(t *testing.T) {
	handler := NewHandler(&config.Config{HTTPPort: "8080"}, "HTTP")
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/config.json", nil))
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Contains(t, response.Body.String(), `"http_port":"8080"`)
		}()
	}
	wg.Wait()
}

func TestBareUIRedirectPreservesProxyPrefix(t *testing.T) {
	handler := NewHandler(&config.Config{}, "HTTP")
	server := httptest.NewServer(http.StripPrefix("/prefix", handler))
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for _, test := range []struct{ path, location string }{
		{"ui", "ui/"},
		{"ui/workbench", "workbench/"},
		{"ui/examples", "examples/"},
		{"ui/connect", "connect/"},
		{"ui/privacy", "privacy/"},
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			req, err := http.NewRequest(method, server.URL+"/prefix/"+test.path+"?tab=websocket", nil)
			require.NoError(t, err)
			response, err := client.Do(req)
			require.NoError(t, err)
			_ = response.Body.Close()
			assert.Equal(t, http.StatusMovedPermanently, response.StatusCode)
			assert.Equal(t, test.location+"?tab=websocket", response.Header.Get("Location"))
			target, err := response.Location()
			require.NoError(t, err)
			assert.Equal(t, "/prefix/"+test.path+"/", target.Path)
			assert.Equal(t, "tab=websocket", target.RawQuery)
			assert.Equal(t, "nosniff", response.Header.Get("X-Content-Type-Options"))
			assert.Equal(t, "no-store", response.Header.Get("Cache-Control"))
		}
	}
}
