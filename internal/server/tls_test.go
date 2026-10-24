package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	apphealth "github.com/PhilipSchmid/echo-app/internal/health"
	pb "github.com/PhilipSchmid/echo-app/proto"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type tlsFixture struct {
	cfg    config.Config
	roots  *x509.CertPool
	client tls.Certificate
}

func newTLSFixture(t *testing.T) tlsFixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	require.NoError(t, err)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	issue := func(serial int64, usage x509.ExtKeyUsage) (tls.Certificate, []byte, []byte) {
		leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		leaf := &x509.Certificate{SerialNumber: big.NewInt(serial), DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
		leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
		require.NoError(t, err)
		keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
		require.NoError(t, err)
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		require.NoError(t, err)
		return cert, certPEM, keyPEM
	}
	_, certPEM, keyPEM := issue(2, x509.ExtKeyUsageServerAuth)
	client, _, _ := issue(3, x509.ExtKeyUsageClientAuth)
	dir := t.TempDir()
	certFile, keyFile, caFile := filepath.Join(dir, "server.pem"), filepath.Join(dir, "server.key"), filepath.Join(dir, "ca.pem")
	require.NoError(t, os.WriteFile(certFile, certPEM, 0600))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0600))
	require.NoError(t, os.WriteFile(caFile, caPEM, 0600))
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(caPEM))
	return tlsFixture{cfg: config.Config{TLS: true, QUIC: true, GRPC: true, GRPCTLS: true, TLSPort: "0", QUICPort: "0", GRPCPort: "0", MaxRequestSize: 1024, TLSCertFile: certFile, TLSKeyFile: keyFile, TLSClientCAFile: caFile}, roots: roots, client: client}
}

func TestTLSListenersVerifyBothPeers(t *testing.T) {
	fixture := newTLSFixture(t)
	untrusted := newTLSFixture(t)
	for _, protocol := range []string{"HTTPS", "HTTP2", "HTTP3", "gRPC"} {
		t.Run(protocol, func(t *testing.T) {
			var srv Server
			var address func() string
			switch protocol {
			case "HTTP3":
				q := NewQUICServer(&fixture.cfg)
				q.listenAddr = "127.0.0.1:0"
				srv = q
				address = func() string { return q.packet.LocalAddr().String() }
			case "gRPC":
				g := NewGRPCServer(&fixture.cfg)
				g.listenAddr = "127.0.0.1:0"
				srv = g
				address = func() string { return g.listener.Addr().String() }
			default:
				h := NewHTTPServer(&fixture.cfg, true)
				h.listenAddr = "127.0.0.1:0"
				srv = h
				address = func() string { return h.netListener.Addr().String() }
			}
			done := make(chan error, 1)
			go func() { done <- srv.Start(context.Background()) }()
			select {
			case <-srv.Ready():
			case err := <-done:
				t.Fatalf("TLS listener failed: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("TLS listener failed to bind")
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				require.NoError(t, srv.Shutdown(ctx))
				serveErr := <-done
				if protocol == "gRPC" {
					require.NoError(t, serveErr)
				} else {
					require.ErrorIs(t, serveErr, http.ErrServerClosed)
				}
			}()
			request := func(clientTLS *tls.Config) error {
				if protocol == "gRPC" {
					conn, err := grpc.NewClient(address(), grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
					if err != nil {
						return err
					}
					defer func() { _ = conn.Close() }()
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					defer cancel()
					_, err = pb.NewEchoServiceClient(conn).Echo(ctx, &pb.EchoRequest{})
					return err
				}
				var transport http.RoundTripper
				if protocol == "HTTP3" {
					tr := &http3.Transport{TLSClientConfig: clientTLS}
					defer func() { _ = tr.Close() }()
					transport = tr
				} else {
					tr := &http.Transport{TLSClientConfig: clientTLS, ForceAttemptHTTP2: protocol == "HTTP2"}
					defer tr.CloseIdleConnections()
					transport = tr
				}
				client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
				resp, err := client.Get("https://" + address())
				if err != nil {
					return err
				}
				defer func() { _ = resp.Body.Close() }()
				switch protocol {
				case "HTTP3":
					require.Equal(t, 3, resp.ProtoMajor)
				case "HTTP2":
					require.Equal(t, 2, resp.ProtoMajor)
				}
				_, err = io.Copy(io.Discard, resp.Body)
				return err
			}
			valid := &tls.Config{RootCAs: fixture.roots, ServerName: "localhost", Certificates: []tls.Certificate{fixture.client}, MinVersion: tls.VersionTLS12}
			require.NoError(t, request(valid))
			missing := valid.Clone()
			missing.Certificates = nil
			require.Error(t, request(missing), "client certificate is required")
			wrongClient := valid.Clone()
			wrongClient.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &untrusted.client, nil }
			require.Error(t, request(wrongClient), "client certificate must chain to configured CA")
			wrongName := valid.Clone()
			wrongName.ServerName = "wrong.invalid"
			require.Error(t, request(wrongName), "client must verify server DNS name")
			wrongRoots := valid.Clone()
			wrongRoots.RootCAs = untrusted.roots
			require.Error(t, request(wrongRoots), "client must trust the server CA")
		})
	}
}

func TestGRPCHealthTracksReadinessAndFinishesWatches(t *testing.T) {
	checker := apphealth.NewChecker(config.ExternalReadinessProbe{})
	checker.SetReady(false, "starting")
	srv := NewGRPCServer(&config.Config{GRPCPort: "0"}, checker)
	srv.listenAddr = "127.0.0.1:0"
	done := make(chan error, 1)
	go func() { done <- srv.Start(context.Background()) }()
	<-srv.Ready()
	conn, err := grpc.NewClient(srv.listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := healthpb.NewHealthClient(conn)
	for _, name := range []string{"", "echo.EchoService"} {
		response, err := client.Check(ctx, &healthpb.HealthCheckRequest{Service: name})
		require.NoError(t, err)
		require.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, response.Status)
	}
	_, err = client.Check(ctx, &healthpb.HealthCheckRequest{Service: "unknown"})
	require.Equal(t, codes.NotFound, status.Code(err))
	unknownWatch, err := client.Watch(ctx, &healthpb.HealthCheckRequest{Service: "unknown"})
	require.NoError(t, err)
	unknownResponse, err := unknownWatch.Recv()
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVICE_UNKNOWN, unknownResponse.Status)
	watch, err := client.Watch(ctx, &healthpb.HealthCheckRequest{Service: "echo.EchoService"})
	require.NoError(t, err)
	response, err := watch.Recv()
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, response.Status)
	checker.SetReady(true, "")
	response, err = watch.Recv()
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, response.Status)
	checker.SetHealthy(false, "unhealthy")
	response, err = watch.Recv()
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, response.Status)
	checker.SetHealthy(true, "")
	response, err = watch.Recv()
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, response.Status)
	// Readiness may already be false before shutdown starts; the watch must still finish.
	checker.SetReady(false, "draining")
	response, err = watch.Recv()
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, response.Status)
	stopped := make(chan error, 1)
	go func() { stopped <- srv.Shutdown(ctx) }()
	response, err = watch.Recv()
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_NOT_SERVING, response.Status)
	_, err = watch.Recv()
	require.ErrorIs(t, err, io.EOF)
	_, err = unknownWatch.Recv()
	require.ErrorIs(t, err, io.EOF)
	require.NoError(t, <-stopped)
	require.NoError(t, <-done)
}
