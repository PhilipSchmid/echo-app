package server

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/quic-go/quicvarint"
	"github.com/stretchr/testify/require"
)

func TestHTTP3ConnectionLifetimeBoundsActiveIncompleteHeaders(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "protection enabled"
		if !enabled {
			name = "protection disabled"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newTLSFixture(t)
			fixture.cfg.TLSClientCAFile = ""
			srv := NewQUICServer(&fixture.cfg, nil)
			require.Equal(t, 2*time.Minute, srv.connectionLifetime)
			srv.connectionLifetime = 200 * time.Millisecond
			if enabled {
				guard, err := protection.NewGuard(protection.Options{
					Enabled: true, RequestRate: 1000, IngressRate: 1 << 20, EgressRate: 1 << 20, DailyEgress: 1 << 30,
				})
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, guard.Close()) })
				srv.SetProtection(guard)
			}
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
			clientTLS := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: fixture.roots, ServerName: "localhost", NextProtos: []string{http3.NextProtoH3}}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, err := quic.DialAddr(ctx, srv.packet.LocalAddr().String(), clientTLS, &quic.Config{KeepAlivePeriod: 10 * time.Millisecond})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, conn.CloseWithError(0, "test complete")) })
			control, err := conn.OpenUniStreamSync(ctx)
			require.NoError(t, err)
			_, err = control.Write([]byte{0, 4, 0})
			require.NoError(t, err)
			stream, err := conn.OpenStreamSync(ctx)
			require.NoError(t, err)
			_, err = stream.Write(quicvarint.Append(quicvarint.Append(nil, 1), maxHTTP3HeaderBytes))
			require.NoError(t, err)
			require.Eventually(t, func() bool { return len(srv.connections) == 1 }, 100*time.Millisecond, time.Millisecond)
			// Continue sending an unfinished header block, so inactivity cannot explain closure.
			trafficDone := make(chan struct{})
			go func() {
				defer close(trafficDone)
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-conn.Context().Done():
						return
					case <-ticker.C:
						if _, err := stream.Write([]byte{0}); err != nil {
							return
						}
					}
				}
			}()
			t.Cleanup(func() {
				_ = conn.CloseWithError(0, "test complete")
				select {
				case <-trafficDone:
				case <-time.After(time.Second):
					t.Error("header traffic did not stop")
				}
			})
			if enabled {
				select {
				case <-conn.Context().Done():
				case <-time.After(2 * time.Second):
					t.Fatal("active incomplete headers exceeded the connection lifetime")
				}
				var closed *quic.ApplicationError
				require.ErrorAs(t, context.Cause(conn.Context()), &closed)
				require.Equal(t, quic.ApplicationErrorCode(http3.ErrCodeNoError), closed.ErrorCode)
				require.Equal(t, "connection lifetime reached", closed.ErrorMessage)
				require.Eventually(t, func() bool { return len(srv.connections) == 0 }, time.Second, time.Millisecond)
			} else {
				select {
				case <-conn.Context().Done():
					t.Fatalf("disabled protection changed connection lifetime: %v", context.Cause(conn.Context()))
				case <-time.After(2 * srv.connectionLifetime):
				}
			}
			transport := &http3.Transport{TLSClientConfig: clientTLS}
			t.Cleanup(func() { require.NoError(t, transport.Close()) })
			client := &http.Client{Transport: transport, Timeout: time.Second}
			response, err := client.Get("https://" + srv.packet.LocalAddr().String() + "/")
			require.NoError(t, err, "another connection must still serve requests")
			_, err = io.Copy(io.Discard, response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusOK, response.StatusCode)
		})
	}
}
