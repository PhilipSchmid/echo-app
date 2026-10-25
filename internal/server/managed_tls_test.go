package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	pb "github.com/PhilipSchmid/echo-observatory/v2/proto"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/acme"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func TestSharedTLSProviderRotatesAcrossListeners(t *testing.T) {
	first, second := newTLSFixture(t), newTLSFixture(t)
	certificates := make([]tls.Certificate, 2)
	for i, fixture := range []tlsFixture{first, second} {
		cert, err := tls.LoadX509KeyPair(fixture.cfg.TLSCertFile, fixture.cfg.TLSKeyFile)
		require.NoError(t, err)
		certificates[i] = cert
	}
	otherCA, err := os.ReadFile(second.cfg.TLSClientCAFile)
	require.NoError(t, err)
	require.True(t, first.roots.AppendCertsFromPEM(otherCA))
	var current atomic.Pointer[tls.Certificate]
	current.Store(&certificates[0])
	var handshakes atomic.Int32
	shared := &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2", "http/1.1", acme.ALPNProto},
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if hello.ServerName != "localhost" {
				return nil, fmt.Errorf("unexpected SNI %q", hello.ServerName)
			}
			handshakes.Add(1)
			return current.Load(), nil
		},
	}
	for _, protocol := range []string{"HTTPS", "HTTP2", "gRPC", "HTTP3"} {
		t.Run(protocol, func(t *testing.T) {
			cfg := &config.Config{TLSPort: "0", GRPCPort: "0", GRPCTLS: true, QUICPort: "0", QUICHost: "127.0.0.1", MaxRequestSize: 1024}
			var srv Server
			var address func() string
			switch protocol {
			case "HTTP3":
				q := NewQUICServer(cfg, shared)
				srv, address = q, func() string { return q.packet.LocalAddr().String() }
			case "gRPC":
				g := NewGRPCServer(cfg, nil, shared)
				g.listenAddr = "127.0.0.1:0"
				srv, address = g, func() string { return g.listener.Addr().String() }
			default:
				h := NewHTTPServer(cfg, true, shared)
				h.listenAddr = "127.0.0.1:0"
				srv, address = h, func() string { return h.netListener.Addr().String() }
			}
			done := make(chan error, 1)
			go func() { done <- srv.Start(context.Background()) }()
			select {
			case <-srv.Ready():
			case err := <-done:
				t.Fatalf("listener failed: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("listener failed to bind")
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				require.NoError(t, srv.Shutdown(ctx))
				err := <-done
				if protocol == "gRPC" {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, http.ErrServerClosed)
				}
			})
			for i := range certificates {
				current.Store(&certificates[i])
				before := handshakes.Load()
				clientTLS := &tls.Config{
					MinVersion: tls.VersionTLS12, RootCAs: first.roots, ServerName: "localhost",
					VerifyConnection: func(state tls.ConnectionState) error {
						if !bytes.Equal(state.PeerCertificates[0].Raw, certificates[i].Certificate[0]) {
							return fmt.Errorf("listener served the old certificate")
						}
						return nil
					},
				}
				if protocol == "gRPC" {
					conn, err := grpc.NewClient(address(), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
					require.NoError(t, err)
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					_, err = pb.NewEchoServiceClient(conn).Echo(ctx, &pb.EchoRequest{})
					cancel()
					require.NoError(t, conn.Close())
					require.NoError(t, err)
				} else {
					var transport http.RoundTripper
					var closeTransport func()
					if protocol == "HTTP3" {
						tr := &http3.Transport{TLSClientConfig: clientTLS}
						transport, closeTransport = tr, func() { require.NoError(t, tr.Close()) }
					} else {
						tr := &http.Transport{TLSClientConfig: clientTLS, ForceAttemptHTTP2: protocol == "HTTP2"}
						transport, closeTransport = tr, tr.CloseIdleConnections
					}
					client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
					response, err := client.Get("https://" + address())
					require.NoError(t, err)
					require.Equal(t, http.StatusOK, response.StatusCode)
					major := 1
					switch protocol {
					case "HTTP2":
						major = 2
					case "HTTP3":
						major = 3
					}
					require.Equal(t, major, response.ProtoMajor)
					_, err = io.Copy(io.Discard, response.Body)
					require.NoError(t, response.Body.Close())
					closeTransport()
					require.NoError(t, err)
				}
				require.Greater(t, handshakes.Load(), before, "listener must use the shared callback on each new connection")
			}
			if protocol == "HTTPS" {
				conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", address(), &tls.Config{RootCAs: first.roots, ServerName: "localhost", MinVersion: tls.VersionTLS12, NextProtos: []string{acme.ALPNProto}})
				require.NoError(t, err)
				require.Equal(t, acme.ALPNProto, conn.ConnectionState().NegotiatedProtocol)
				require.NoError(t, conn.Close())
			}
		})
	}
	require.Equal(t, []string{"h2", "http/1.1", acme.ALPNProto}, shared.NextProtos, "listeners must not mutate the shared provider")
}
