package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	"github.com/stretchr/testify/require"
)

func readServerCertificate(t *testing.T, handler http.Handler) *handlers.ServerCertificate {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/certificate.json", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var document struct {
		Certificate *handlers.ServerCertificate `json:"certificate"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &document))
	return document.Certificate
}

func TestCertificateUIUsesSharedSourceWithoutCallingTLSProvider(t *testing.T) {
	first, second := newTLSFixture(t), newTLSFixture(t)
	var current atomic.Pointer[tls.Certificate]
	var calls atomic.Int32
	shared := &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			calls.Add(1)
			return current.Load(), nil
		},
	}
	source := handlers.TrackServerCertificate(shared)
	cfg := &config.Config{UI: true, TLS: true, QUIC: true}
	plain := NewHTTPServer(cfg, false, nil, source)
	encrypted := NewHTTPServer(cfg, true, shared, source)
	quic := NewQUICServer(cfg, shared, source)
	t.Cleanup(plain.closeWebSockets)
	t.Cleanup(encrypted.closeWebSockets)
	uiHandlers := []http.Handler{plain.server.Handler, encrypted.server.Handler, quic.server.Handler}
	for _, handler := range uiHandlers {
		require.Nil(t, readServerCertificate(t, handler))
	}
	require.Zero(t, calls.Load(), "UI reads must not start certificate issuance")
	var previous *handlers.ServerCertificate
	for _, fixture := range []tlsFixture{first, second} {
		certificate, err := tls.LoadX509KeyPair(fixture.cfg.TLSCertFile, fixture.cfg.TLSKeyFile)
		require.NoError(t, err)
		current.Store(&certificate)
		_, err = shared.GetCertificate(&tls.ClientHelloInfo{ServerName: "localhost"})
		require.NoError(t, err)
		expected := source()
		require.NotNil(t, expected)
		require.NotEqual(t, previous, expected, "normal certificate rotation must reach the shared UI source")
		before := calls.Load()
		for _, handler := range uiHandlers {
			require.Equal(t, expected, readServerCertificate(t, handler))
		}
		require.Equal(t, before, calls.Load(), "UI reads must only inspect the observed certificate")
		previous = expected
	}
}

func TestCertificateUITracksStandaloneListenerAfterStart(t *testing.T) {
	fixture := newTLSFixture(t)
	certificate, err := tls.LoadX509KeyPair(fixture.cfg.TLSCertFile, fixture.cfg.TLSKeyFile)
	require.NoError(t, err)
	expected := handlers.TrackServerCertificate(&tls.Config{Certificates: []tls.Certificate{certificate}})()
	for _, protocol := range []string{"HTTPS", "HTTP3"} {
		t.Run(protocol, func(t *testing.T) {
			cfg := fixture.cfg
			cfg.UI, cfg.QUICHost = true, "127.0.0.1"
			var srv Server
			var handler http.Handler
			if protocol == "HTTP3" {
				quic := NewQUICServer(&cfg, nil)
				srv, handler = quic, quic.server.Handler
			} else {
				https := NewHTTPServer(&cfg, true, nil)
				https.listenAddr = "127.0.0.1:0"
				srv, handler = https, https.server.Handler
			}
			require.Nil(t, readServerCertificate(t, handler))
			stopReads := make(chan struct{})
			var readers sync.WaitGroup
			readers.Go(func() {
				for {
					select {
					case <-stopReads:
						return
					default:
						response := httptest.NewRecorder()
						handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/certificate.json", nil))
					}
				}
			})
			done := make(chan error, 1)
			returned := false
			go func() { done <- srv.Start(context.Background()) }()
			t.Cleanup(func() {
				close(stopReads)
				readers.Wait()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				require.NoError(t, srv.Shutdown(ctx))
				if returned {
					return
				}
				select {
				case err := <-done:
					require.ErrorIs(t, err, http.ErrServerClosed)
				case <-time.After(3 * time.Second):
					t.Fatal("listener did not stop")
				}
			})
			select {
			case <-srv.Ready():
			case err := <-done:
				returned = true
				t.Fatalf("listener failed: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("listener failed to bind")
			}
			require.Equal(t, expected, readServerCertificate(t, handler))
		})
	}
}
