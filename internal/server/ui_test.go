package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTP3ReusesProtectedConnectionForAllUIPagesAndAssets(t *testing.T) {
	fixture := newTLSFixture(t)
	fixture.cfg.UI, fixture.cfg.TLSClientCAFile, fixture.cfg.PublicURL = true, "", "https://localhost/"
	guard, err := protection.NewGuard(protection.Options{
		Enabled: true, RequestRate: 20, PeerRate: 5, IngressRate: 256 << 10, EgressRate: 1 << 20, DailyEgress: 1 << 30,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, guard.Close()) })
	srv := NewQUICServer(&fixture.cfg, nil)
	srv.SetProtection(guard)
	srv.listenAddr = "127.0.0.1:0"
	done := make(chan error, 1)
	go func() { done <- srv.Start(context.Background()) }()
	select {
	case <-srv.Ready():
	case err := <-done:
		t.Fatalf("HTTP/3 listener failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP/3 listener did not bind")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
		require.ErrorIs(t, <-done, http.ErrServerClosed)
	})
	var dials atomic.Int32
	transport := &http3.Transport{
		TLSClientConfig:    &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: fixture.roots, ServerName: "localhost"},
		DisableCompression: true,
		Dial: func(ctx context.Context, address string, tlsConfig *tls.Config, quicConfig *quic.Config) (*quic.Conn, error) {
			if dials.Add(1) != 1 {
				return nil, errors.New("unexpected QUIC redial")
			}
			return quic.DialAddrEarly(ctx, address, tlsConfig, quicConfig)
		},
	}
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	remaining := int64(1 << 20)
	for _, path := range []string{
		"/ui/", "/ui/workbench/", "/ui/examples/", "/ui/connect/", "/ui/privacy/", "/ui/config.json", "/ui/certificate.json",
		"/ui/certificate.js", "/ui/app.js", "/ui/common.js", "/ui/controls.js", "/ui/headers.js", "/ui/visit.js", "/ui/guide.js",
		"/ui/presets.js", "/ui/highlight.js", "/ui/terminal.js", "/ui/privacy.js", "/ui/app.css", "/ui/logo.svg", "/ui/favicon.svg",
		"/ui/nunito-latin.woff2", "/ui/sometype-mono-latin.woff2", "/robots.txt", "/sitemap.xml",
	} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+srv.packet.LocalAddr().String()+path, nil)
		require.NoError(t, err)
		response, err := client.Do(req)
		require.NoError(t, err, path)
		n, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, remaining+1))
		closeErr := response.Body.Close()
		require.NoError(t, readErr, path)
		require.NoError(t, closeErr, path)
		require.Positive(t, n, path)
		require.LessOrEqual(t, n, remaining, "UI transfer exceeded 1 MiB")
		remaining -= n
		require.Equal(t, http.StatusOK, response.StatusCode, path)
		require.Equal(t, 3, response.ProtoMajor, path)
		require.NotNil(t, response.TLS, path)
		require.NotEmpty(t, response.TLS.VerifiedChains, path)
		require.Equal(t, http3.NextProtoH3, response.TLS.NegotiatedProtocol, path)
		if response.ContentLength >= 0 {
			require.Equal(t, response.ContentLength, n, path)
		}
		select {
		case <-time.After(220 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal("HTTP/3 UI sequence exceeded its time limit")
		}
	}
	require.Equal(t, int32(1), dials.Load(), "every response must use the same QUIC connection")
}

func TestUIRoutesPreserveEchoAndWebSocket(t *testing.T) {
	for _, listener := range []string{"HTTP", "TLS", "QUIC"} {
		for _, enabled := range []bool{false, true} {
			cfg := &config.Config{UI: enabled, PublicURL: "https://echo.container.education/", HTTPPort: "8080", TLSPort: "8443", MaxRequestSize: 1024}
			var handler http.Handler
			if listener == "QUIC" {
				handler = NewQUICServer(cfg, nil).server.Handler
			} else {
				srv := NewHTTPServer(cfg, listener == "TLS", nil)
				t.Cleanup(func() { srv.closeWebSockets() })
				handler = srv.server.Handler
			}
			for _, path := range []string{"/", "/example?item=42", "/ui/", "/ui/workbench/", "/ui/examples/", "/ui/connect/", "/ui/privacy/", "/ui/app.js", "/ui/headers.js", "/ui/visit.js", "/ui/highlight.js", "/ui/terminal.js", "/ui/common.js", "/ui/presets.js", "/ui/guide.js", "/ui/controls.js", "/ui/privacy.js", "/ui/logo.svg", "/ui/favicon.svg", "/ui/nunito-latin.woff2", "/ui/sometype-mono-latin.woff2", "/ui/config.json", "/ui/certificate.json", "/robots.txt", "/sitemap.xml"} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
				require.Equal(t, http.StatusOK, response.Code)
				if enabled && (path != "/" && path != "/example?item=42") {
					switch path {
					case "/ui/", "/ui/workbench/", "/ui/examples/", "/ui/connect/", "/ui/privacy/":
						assert.Contains(t, response.Header().Get("Content-Type"), "text/html")
						privacyLink := `href="../privacy/"`
						if path == "/ui/" {
							privacyLink = `href="./privacy/"`
						}
						assert.Contains(t, response.Body.String(), privacyLink)
						if path == "/ui/privacy/" {
							assert.Contains(t, response.Body.String(), "<title>Privacy &amp; data — Echo Observatory</title>")
							assert.Contains(t, response.Body.String(), "<h1>Privacy &amp; data</h1>")
							assert.Contains(t, response.Body.String(), `src="../privacy.js"`)
						}
					case "/robots.txt":
						assert.Contains(t, response.Body.String(), "Sitemap: https://echo.container.education/sitemap.xml")
					case "/sitemap.xml":
						assert.Contains(t, response.Body.String(), "https://echo.container.education/ui/")
						assert.Contains(t, response.Body.String(), "https://echo.container.education/ui/privacy/")
					case "/ui/certificate.json":
						assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
						assert.Contains(t, response.Body.String(), `"certificate":null`)
					case "/ui/config.json":
						assert.Contains(t, response.Body.String(), `"http_port":"8080"`)
					case "/ui/logo.svg", "/ui/favicon.svg":
						assert.Equal(t, "image/svg+xml", response.Header().Get("Content-Type"))
					case "/ui/nunito-latin.woff2", "/ui/sometype-mono-latin.woff2":
						assert.Equal(t, "font/woff2", response.Header().Get("Content-Type"))
					default:
						assert.Contains(t, response.Header().Get("Content-Type"), "text/javascript")
					}
					continue
				}
				assert.Equal(t, "noindex", response.Header().Get("X-Robots-Tag"))
				if enabled && path == "/" {
					assert.Contains(t, response.Header().Get("Link"), `<ui/>; rel="alternate"; type="text/html"`)
				} else {
					assert.Empty(t, response.Header().Get("Link"))
				}
				var echo handlers.HTTPResponse
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &echo))
				assert.Equal(t, listener, echo.Listener)
				assert.NotEmpty(t, echo.Hostname)
			}
			missing := httptest.NewRecorder()
			handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/ui/missing", nil))
			if enabled {
				assert.Equal(t, http.StatusNotFound, missing.Code)
			} else {
				assert.Equal(t, http.StatusOK, missing.Code)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ws", nil))
			assert.Equal(t, http.StatusUpgradeRequired, response.Code)
			for _, entry := range []struct{ path, location string }{
				{"/ui", "ui/"},
				{"/ui/examples", "examples/"},
				{"/ui/workbench", "workbench/"},
				{"/ui/connect", "connect/"},
				{"/ui/privacy", "privacy/"},
			} {
				response = httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, entry.path, nil))
				if enabled {
					assert.Equal(t, http.StatusMovedPermanently, response.Code)
					assert.Equal(t, entry.location, response.Header().Get("Location"))
				} else {
					assert.Equal(t, http.StatusOK, response.Code)
					var echo handlers.HTTPResponse
					require.NoError(t, json.Unmarshal(response.Body.Bytes(), &echo))
					assert.Equal(t, listener, echo.Listener)
				}
			}
		}
	}
}

func TestHTTP3AdvertisementRequiresExplicitPublicPort(t *testing.T) {
	for _, tc := range []struct {
		name       string
		tls, quic  bool
		port, want string
	}{
		{"plain HTTP", false, true, "443", ""},
		{"QUIC off", true, false, "443", ""},
		{"no public mapping", true, true, "", ""},
		{"standard mapping", true, true, "443", `h3=":443"; ma=300`},
		{"custom mapping", true, true, "19443", `h3=":19443"; ma=300`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{UI: true, QUIC: tc.quic, QUICPort: "4433", QUICAdvertisePort: tc.port}
			srv := NewHTTPServer(cfg, tc.tls, nil)
			t.Cleanup(srv.closeWebSockets)
			for _, path := range []string{"/", "/ui/", "/ui/privacy/", "/ui/app.js", "/ui/privacy.js", "/ws"} {
				response := httptest.NewRecorder()
				srv.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
				assert.Equal(t, tc.want, response.Header().Get("Alt-Svc"), path)
			}
		})
	}
}
