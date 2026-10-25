package server

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/require"
)

type deadlineWriter struct {
	*httptest.ResponseRecorder
	readDeadline, writeDeadline time.Time
	readError, writeError       error
}

func (w *deadlineWriter) SetReadDeadline(deadline time.Time) error {
	w.readDeadline = deadline
	return w.readError
}

func (w *deadlineWriter) SetWriteDeadline(deadline time.Time) error {
	w.writeDeadline = deadline
	return w.writeError
}

type checkingReader func([]byte) (int, error)

func (read checkingReader) Read(data []byte) (int, error) { return read(data) }

func TestHTTP3DeadlinesCoverEveryRouteBeforeReadingBody(t *testing.T) {
	srv := NewQUICServer(&config.Config{UI: true, PublicURL: "https://echo.example/", MaxRequestSize: 4}, nil)
	require.Equal(t, 30*time.Second, srv.streamTimeout)
	for _, path := range []string{"/", "/ui/", "/ui/app.js", "/robots.txt", "/sitemap.xml", "/ws"} {
		t.Run(path, func(t *testing.T) {
			w := &deadlineWriter{ResponseRecorder: httptest.NewRecorder()}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.ProtoMajor = 3
			req.Body = io.NopCloser(checkingReader(func([]byte) (int, error) {
				require.WithinDuration(t, time.Now().Add(srv.streamTimeout), w.readDeadline, time.Second)
				require.Equal(t, w.readDeadline, w.writeDeadline)
				return 0, io.EOF
			}))
			srv.server.Handler.ServeHTTP(w, req)
			require.WithinDuration(t, time.Now().Add(srv.streamTimeout), w.readDeadline, time.Second)
			require.Equal(t, w.readDeadline, w.writeDeadline)
		})
	}
	t.Run("overload rejection", func(t *testing.T) {
		atomic.StoreInt32(&srv.activeRequests, maxConcurrentRequests)
		w := &deadlineWriter{ResponseRecorder: httptest.NewRecorder()}
		srv.server.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ui/", nil))
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
		require.WithinDuration(t, time.Now().Add(srv.streamTimeout), w.writeDeadline, time.Second)
		require.Equal(t, w.readDeadline, w.writeDeadline)
	})
}

func TestHTTP3DeadlineErrorsAreObserved(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		readError, writeError error
		status                int
	}{
		{"unsupported recorder", http.ErrNotSupported, http.ErrNotSupported, http.StatusOK},
		{"read deadline failure", errors.New("read failed"), nil, http.StatusInternalServerError},
		{"write deadline failure", nil, errors.New("write failed"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := NewQUICServer(&config.Config{UI: true, MaxRequestSize: 4}, nil)
			responses := metrics.HTTPResponsesTotal.WithLabelValues("QUIC", "/ui/", strconv.Itoa(tc.status))
			before := testutil.ToFloat64(responses)
			w := &deadlineWriter{ResponseRecorder: httptest.NewRecorder(), readError: tc.readError, writeError: tc.writeError}
			req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
			req.ProtoMajor = 3
			srv.server.Handler.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, before+1, testutil.ToFloat64(responses))
		})
	}
}

func TestHTTP3SlowUIReadersReleaseRequestCapacity(t *testing.T) {
	fixture := newTLSFixture(t)
	fixture.cfg.UI, fixture.cfg.TLSClientCAFile = true, ""
	srv := NewQUICServer(&fixture.cfg, nil)
	srv.streamTimeout = 250 * time.Millisecond
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
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: fixture.roots, ServerName: "localhost"},
		Dial: func(ctx context.Context, address string, tlsConfig *tls.Config, quicConfig *quic.Config) (*quic.Conn, error) {
			dials.Add(1)
			return quic.DialAddrEarly(ctx, address, tlsConfig, quicConfig)
		},
		QUICConfig: &quic.Config{
			InitialStreamReceiveWindow: 1024,
			MaxStreamReceiveWindow:     1024,
			KeepAlivePeriod:            10 * time.Millisecond,
		},
	}
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	address := "https://" + srv.packet.LocalAddr().String()
	for _, path := range []string{"/ui/", "/ui/nunito-latin.woff2"} {
		t.Run(path, func(t *testing.T) {
			response, err := client.Get(address + path)
			require.NoError(t, err)
			defer func() { require.NoError(t, response.Body.Close()) }()
			require.Equal(t, http.StatusOK, response.StatusCode)
			// Leave the body unread while QUIC keepalives prevent idle connection cleanup.
			require.Eventually(t, func() bool { return atomic.LoadInt32(&srv.activeRequests) == 1 }, time.Second, time.Millisecond)
			require.Eventually(t, func() bool { return atomic.LoadInt32(&srv.activeRequests) == 0 }, 2*time.Second, time.Millisecond)
		})
	}
	response, err := client.Get(address + "/")
	require.NoError(t, err, "a stream timeout must not close the reusable QUIC connection")
	_, err = io.Copy(io.Discard, response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, int32(1), dials.Load(), "stream deadlines must preserve the existing QUIC connection")
}
