package server

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/quic-go/quicvarint"
	"github.com/stretchr/testify/require"
)

func startHardenedQUICTestServer(t *testing.T) (*QUICServer, *tls.Config) {
	t.Helper()
	fixture := newTLSFixture(t)
	fixture.cfg.TLSClientCAFile = ""
	srv := NewQUICServer(&fixture.cfg, nil)
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
	return srv, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: fixture.roots, ServerName: "localhost", NextProtos: []string{http3.NextProtoH3}}
}

func TestHTTP3HeaderSizeIsBounded(t *testing.T) {
	srv, clientTLS := startHardenedQUICTestServer(t)
	transport := &http3.Transport{TLSClientConfig: clientTLS}
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	for _, test := range []struct{ size, status int }{{32 << 10, http.StatusOK}, {64 << 10, http.StatusRequestHeaderFieldsTooLarge}, {0, http.StatusOK}} {
		req, err := http.NewRequest(http.MethodGet, "https://"+srv.packet.LocalAddr().String()+"/", nil)
		require.NoError(t, err)
		req.Header.Set("X-Example", strings.Repeat("a", test.size))
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, test.status, resp.StatusCode)
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
}

func TestHTTP3IncompleteHeadersRespectStreamLimit(t *testing.T) {
	srv, clientTLS := startHardenedQUICTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := quic.DialAddr(ctx, srv.packet.LocalAddr().String(), clientTLS, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.CloseWithError(0, "test complete")) })
	control, err := conn.OpenUniStreamSync(ctx)
	require.NoError(t, err)
	_, err = control.Write([]byte{0, 4, 0}) // Control stream with an empty SETTINGS frame.
	require.NoError(t, err)
	frame := quicvarint.Append(quicvarint.Append(nil, 1), maxHTTP3HeaderBytes)
	for range maxQUICStreams {
		stream, err := conn.OpenStreamSync(ctx)
		require.NoError(t, err)
		_, err = stream.Write(frame) // Announce a bounded header block without sending its contents.
		require.NoError(t, err)
	}
	limited, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stop()
	_, err = conn.OpenStreamSync(limited)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	transport := &http3.Transport{TLSClientConfig: clientTLS}
	t.Cleanup(func() { require.NoError(t, transport.Close()) })
	client := &http.Client{Transport: transport, Timeout: time.Second}
	resp, err := client.Get("https://" + srv.packet.LocalAddr().String() + "/")
	require.NoError(t, err, "one connection's incomplete headers must not prevent another client from requesting")
	_, err = io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestQUICDefaultConnectionAdmissionIsBounded(t *testing.T) {
	srv := NewQUICServer(&config.Config{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for range maxQUICConnections {
		_, err := srv.connectionContext(ctx, nil)
		require.NoError(t, err)
	}
	_, err := srv.connectionContext(ctx, nil)
	require.Error(t, err)
	cancel()
	require.Eventually(t, func() bool { return len(srv.connections) == 0 }, time.Second, time.Millisecond)
}
