package ui

import (
	"crypto/tls"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func visitRequest(t *testing.T, response *httptest.ResponseRecorder) handlers.HTTPResponse {
	t.Helper()
	match := regexp.MustCompile(`(?s)<pre id="visit-body"[^>]*>(.*?)</pre>`).FindStringSubmatch(response.Body.String())
	require.Len(t, match, 2, "request must be present in server-rendered HTML without a fetch")
	var request handlers.HTTPResponse
	require.NoError(t, json.Unmarshal([]byte(html.UnescapeString(match[1])), &request))
	return request
}

func TestPageReflectsActualNavigationAndRedactsHeaders(t *testing.T) {
	handler := NewHandler(&config.Config{PrintHeaders: true}, "TLS")
	req := httptest.NewRequest(http.MethodGet, "/ui/?visit=one%2Ftwo", nil)
	req.Host = "echo.example"
	req.RemoteAddr = "192.0.2.8:40500"
	req.Proto, req.ProtoMajor = "HTTP/2.0", 2
	req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, CipherSuite: tls.TLS_AES_128_GCM_SHA256, ServerName: "echo.example", NegotiatedProtocol: "h2"}
	req.Header.Set("Authorization", "Bearer synthetic-secret")
	req.Header.Set("Cookie", "session=synthetic-cookie")
	req.Header.Set("X-Password", "synthetic-password")
	req.Header.Set("X-Forwarded-For", "198.51.100.99")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	require.Equal(t, http.StatusOK, response.Code)
	visit := visitRequest(t, response)
	assert.Equal(t, "GET", visit.HTTPMethod)
	assert.Equal(t, "/ui/?visit=one%2Ftwo", visit.HTTPRequestURI)
	assert.Equal(t, "echo.example", visit.HTTPHost)
	assert.Equal(t, "192.0.2.8", visit.SourceIP, "forwarded headers must not replace the socket peer")
	assert.Equal(t, "HTTP/2.0", visit.HTTPVersion)
	assert.Equal(t, "TLS", visit.Listener)
	assert.Equal(t, "TLS 1.3", visit.TLSVersion)
	assert.Equal(t, "h2", visit.TLSNegotiatedProtocol)
	assert.Equal(t, []string{"[REDACTED]"}, visit.Headers["Authorization"])
	assert.Equal(t, []string{"[REDACTED]"}, visit.Headers["Cookie"])
	assert.Equal(t, []string{"[REDACTED]"}, visit.Headers["X-Password"])
	assert.NotContains(t, response.Body.String(), "synthetic-secret")
	assert.NotContains(t, response.Body.String(), "synthetic-cookie")
	assert.NotContains(t, response.Body.String(), "synthetic-password")
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
}

func TestPageEscapesRequestDataAndOmitsPrivateConfiguration(t *testing.T) {
	const payload = `</pre><svg onload="alert(1)">`
	cfg := &config.Config{PrintHeaders: true, TLSKeyFile: "/private/secret-key", ACMECacheDir: "/private/cache"}
	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	req.Host = payload
	req.Header.Set("X-Demo", payload)
	response := httptest.NewRecorder()
	NewHandler(cfg, "HTTP").ServeHTTP(response, req)
	visit := visitRequest(t, response)
	assert.Equal(t, payload, visit.HTTPHost)
	assert.Equal(t, []string{payload}, visit.Headers["X-Demo"])
	assert.NotContains(t, response.Body.String(), payload)
	assert.NotContains(t, response.Body.String(), cfg.TLSKeyFile)
	assert.NotContains(t, response.Body.String(), cfg.ACMECacheDir)
	assert.NotContains(t, response.Header().Get("Content-Security-Policy"), "unsafe-inline")
}

func TestPageRequestIsNeverReusedForAnotherVisitor(t *testing.T) {
	handler := NewHandler(&config.Config{}, "H2C")
	for _, ip := range []string{"192.0.2.10", "192.0.2.20"} {
		req := httptest.NewRequest(http.MethodGet, "/ui/?visitor="+ip, nil)
		req.RemoteAddr = ip + ":40501"
		req.Header.Set("X-Not-Enabled", "omitted")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		visit := visitRequest(t, response)
		assert.Equal(t, ip, visit.SourceIP)
		assert.True(t, strings.HasSuffix(visit.HTTPRequestURI, ip))
		assert.Equal(t, "HTTP", visit.Listener, "HTTP/1.1 on an H2C-capable listener remains HTTP")
		assert.Empty(t, visit.Headers)
	}
}

func TestPageShowsObservedIPFamilyWithoutTrustingForwardedClaims(t *testing.T) {
	for _, tc := range []struct{ remote, version string }{
		{"[2001:db8::9]:40500", "IPv6"},
		{"192.0.2.9:40500", "IPv4"},
		{"[::ffff:192.0.2.9]:40500", "IPv4"},
		{"unknown", ""},
	} {
		t.Run(tc.remote, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://[::1]:8080/ui/", nil)
			req.RemoteAddr = tc.remote
			req.Header.Set("X-Forwarded-For", "2001:db8::99")
			req.Header.Set("Forwarded", "for=\"[2001:db8::99]\"")
			response := httptest.NewRecorder()
			NewHandler(&config.Config{PrintHeaders: true}, "HTTP").ServeHTTP(response, req)
			visit := visitRequest(t, response)
			assert.Equal(t, tc.version, visit.IPVersion)
			label := tc.version
			if label == "" {
				label = "Unknown IP family"
			}
			assert.Contains(t, response.Body.String(), `<span class="badge neutral ip-family">`+label+`</span>`)
			badge := regexp.MustCompile(`<span class="badge neutral ip-family"([^>]*)>`).FindStringSubmatch(response.Body.String())
			require.Len(t, badge, 2)
			assert.Empty(t, badge[1], "the visible family label needs no tooltip or extra tab stop")
			assert.NotContains(t, response.Body.String(), "IP version used for this request.")
		})
	}
}
