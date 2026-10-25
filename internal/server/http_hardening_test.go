package server

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

func startHTTPBoundaryServer(t *testing.T, srv *HTTPServer) string {
	t.Helper()
	srv.listenAddr = "127.0.0.1:0"
	done := make(chan error, 1)
	go func() { done <- srv.Start(context.Background()) }()
	select {
	case <-srv.Ready():
	case err := <-done:
		t.Fatalf("HTTP listener failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP listener did not bind")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
		require.ErrorIs(t, <-done, http.ErrServerClosed)
	})
	return srv.netListener.Addr().String()
}

func TestHTTPHeadersAreBoundedAcrossProtocols(t *testing.T) {
	fixture := newTLSFixture(t)
	for _, protocol := range []struct {
		name, scheme string
		tls, h2c     bool
		major        int
	}{{"HTTP1", "http", false, false, 1}, {"HTTPS1", "https", true, false, 1}, {"H2C", "http", false, true, 2}, {"HTTPS2", "https", true, false, 2}} {
		t.Run(protocol.name, func(t *testing.T) {
			cfg := fixture.cfg
			cfg.TLSClientCAFile = ""
			cfg.H2C = protocol.h2c
			srv := NewHTTPServer(&cfg, protocol.tls, nil)
			address := startHTTPBoundaryServer(t, srv)
			transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: fixture.roots, ServerName: "localhost"}, ForceAttemptHTTP2: protocol.major == 2}
			if protocol.h2c {
				transport.Protocols = new(http.Protocols)
				transport.Protocols.SetUnencryptedHTTP2(true)
			}
			t.Cleanup(transport.CloseIdleConnections)
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			for _, test := range []struct{ size, status int }{{32 << 10, http.StatusOK}, {80 << 10, http.StatusRequestHeaderFieldsTooLarge}, {0, http.StatusOK}} {
				req, err := http.NewRequest(http.MethodGet, protocol.scheme+"://"+address+"/", nil)
				require.NoError(t, err)
				req.Header.Set("X-Example", strings.Repeat("a", test.size))
				resp, err := client.Do(req)
				if test.status == http.StatusRequestHeaderFieldsTooLarge && protocol.major == 2 && err != nil {
					require.ErrorContains(t, err, "header list", "HTTP/2 clients may enforce the advertised header limit before sending")
					continue
				}
				require.NoError(t, err)
				require.Equal(t, protocol.major, resp.ProtoMajor)
				require.Equal(t, test.status, resp.StatusCode)
				_, err = io.Copy(io.Discard, resp.Body)
				// Oversized HTTP/1 headers can reset the connection after the 431 status.
				if test.status != http.StatusRequestHeaderFieldsTooLarge {
					require.NoError(t, err)
				}
				require.NoError(t, resp.Body.Close())
			}
		})
	}
}

func TestHTTP2SettingsBoundStreamsForTLSAndCleartext(t *testing.T) {
	fixture := newTLSFixture(t)
	for _, useTLS := range []bool{false, true} {
		t.Run(map[bool]string{false: "H2C", true: "TLS"}[useTLS], func(t *testing.T) {
			cfg := fixture.cfg
			cfg.H2C, cfg.TLSClientCAFile = !useTLS, ""
			address := startHTTPBoundaryServer(t, NewHTTPServer(&cfg, useTLS, nil))
			var conn net.Conn
			var err error
			if useTLS {
				conn, err = tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", address, &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: fixture.roots, ServerName: "localhost", NextProtos: []string{"h2"}})
			} else {
				conn, err = net.DialTimeout("tcp", address, time.Second)
			}
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, conn.Close()) })
			require.NoError(t, conn.SetDeadline(time.Now().Add(time.Second)))
			_, err = io.WriteString(conn, http2.ClientPreface)
			require.NoError(t, err)
			framer := http2.NewFramer(conn, conn)
			require.NoError(t, framer.WriteSettings())
			for {
				frame, err := framer.ReadFrame()
				require.NoError(t, err)
				settings, ok := frame.(*http2.SettingsFrame)
				if !ok || settings.IsAck() {
					continue
				}
				limit, present := settings.Value(http2.SettingMaxConcurrentStreams)
				require.True(t, present)
				require.Equal(t, uint32(maxConcurrentRequests), limit)
				break
			}
		})
	}
}

func TestHTTPIncompleteHeadersTimeOut(t *testing.T) {
	srv := NewHTTPServer(&config.Config{MaxRequestSize: 1024}, false, nil)
	require.Equal(t, 5*time.Second, srv.server.ReadHeaderTimeout)
	require.Equal(t, 30*time.Second, srv.server.ReadTimeout)
	srv.server.ReadHeaderTimeout = 30 * time.Millisecond
	conn, err := net.DialTimeout("tcp", startHTTPBoundaryServer(t, srv), time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\nX-Incomplete: ")
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodGet})
	if err == nil {
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusRequestTimeout, response.StatusCode)
	} else {
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	}
}
