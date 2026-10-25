package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicHTTPBrowserSecurityHeaders(t *testing.T) {
	for _, listener := range []string{"HTTP", "H2C", "TLS", "QUIC"} {
		t.Run(listener, func(t *testing.T) {
			cfg := &config.Config{UI: true, H2C: listener == "H2C", HTTPPort: "8080", TLSPort: "8443", MaxRequestSize: 1024}
			var handler http.Handler
			var active *int32
			var setGuard func(*protection.Guard)
			if listener == "QUIC" {
				srv := NewQUICServer(cfg, nil)
				handler, active, setGuard = srv.server.Handler, &srv.activeRequests, srv.SetProtection
			} else {
				srv := NewHTTPServer(cfg, listener == "TLS", nil)
				t.Cleanup(srv.closeWebSockets)
				handler, active, setGuard = srv.server.Handler, &srv.activeRequests, srv.SetProtection
			}
			check := func(method, path string, status int) *httptest.ResponseRecorder {
				t.Helper()
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
				require.Equal(t, status, response.Code, "%s %s", method, path)
				assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
				assert.Equal(t, "no-referrer", response.Header().Get("Referrer-Policy"))
				assert.Equal(t, "DENY", response.Header().Get("X-Frame-Options"))
				policy := response.Header().Get("Permissions-Policy")
				for _, feature := range []string{"camera", "microphone", "geolocation", "payment", "usb"} {
					assert.Contains(t, policy, feature+"=()")
				}
				assert.NotContains(t, policy, "clipboard", "copying must retain the browser's default permission policy")
				assert.Empty(t, response.Header().Get("Strict-Transport-Security"), "plain HTTP remains an intentional diagnostic listener")
				return response
			}
			for _, route := range []struct {
				method, path string
				status       int
			}{
				{http.MethodGet, "/", http.StatusOK},
				{http.MethodHead, "/", http.StatusOK},
				{http.MethodGet, "/ui/", http.StatusOK},
				{http.MethodGet, "/ui/config.json", http.StatusOK},
				{http.MethodGet, "/ui/app.js", http.StatusOK},
				{http.MethodGet, "/ui", http.StatusMovedPermanently},
				{http.MethodGet, "/ui//workbench/", http.StatusTemporaryRedirect},
				{http.MethodPost, "/ui/", http.StatusMethodNotAllowed},
				{http.MethodGet, "/ui/missing", http.StatusNotFound},
				{http.MethodGet, "/ws", http.StatusUpgradeRequired},
			} {
				response := check(route.method, route.path, route.status)
				if route.path == "/ui/" && route.method == http.MethodGet {
					assert.Contains(t, response.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
					assert.NotContains(t, response.Header().Get("Content-Security-Policy"), "unsafe-inline")
				}
			}
			*active = maxConcurrentRequests
			check(http.MethodGet, "/ui/", http.StatusServiceUnavailable)
			*active = 0
			guard, err := protection.NewGuard(protection.Options{Enabled: true, RequestRate: 1, IngressRate: 1024, EgressRate: 1024, DailyEgress: 1024})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, guard.Close()) })
			for range 6 {
				require.NoError(t, guard.AllowOperation("192.0.2.1:1234"))
			}
			setGuard(guard)
			check(http.MethodGet, "/ui/", http.StatusTooManyRequests)
		})
	}
}
