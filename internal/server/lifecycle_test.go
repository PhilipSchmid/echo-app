package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	pb "github.com/PhilipSchmid/echo-observatory/v2/proto"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	reflection "google.golang.org/grpc/reflection/grpc_reflection_v1"
)

func testServers() map[string]func() Server {
	cfg := &config.Config{HTTPPort: "0", TLSPort: "0", TCPPort: "0", GRPCPort: "0", QUICPort: "0", MetricsPort: "0"}
	return map[string]func() Server{
		"HTTP":    func() Server { return NewHTTPServer(cfg, false) },
		"TLS":     func() Server { return NewHTTPServer(cfg, true) },
		"TCP":     func() Server { return NewTCPServer(cfg) },
		"gRPC":    func() Server { return NewGRPCServer(cfg) },
		"QUIC":    func() Server { return NewQUICServer(cfg) },
		"metrics": func() Server { return NewMetricsServer(cfg, nil) },
	}
}

func TestServer_ShutdownBeforeStart(t *testing.T) {
	for name, newServer := range testServers() {
		t.Run(name, func(t *testing.T) {
			srv := newServer()
			require.NoError(t, srv.Shutdown(context.Background()))
			require.Error(t, srv.Start(context.Background()))
		})
	}
}

func TestServer_CanceledStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, newServer := range testServers() {
		t.Run(name, func(t *testing.T) { require.ErrorIs(t, newServer().Start(ctx), context.Canceled) })
	}
}

func TestServer_ConcurrentStartShutdown(t *testing.T) {
	for name, newServer := range testServers() {
		t.Run(name, func(t *testing.T) {
			for range 10 {
				srv := newServer()
				var wg sync.WaitGroup
				wg.Add(2)
				go func() { defer wg.Done(); _ = srv.Start(context.Background()) }()
				go func() {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					_ = srv.Shutdown(ctx)
				}()
				wg.Wait()
			}
		})
	}
}

func TestHTTPServer_DrainsActiveRequest(t *testing.T) {
	srv := NewHTTPServer(&config.Config{HTTPPort: "0"}, false)
	srv.listenAddr = "127.0.0.1:0"
	done := make(chan error, 1)
	entered, release := make(chan struct{}), make(chan struct{})
	srv.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_, _ = w.Write([]byte("finished"))
	})
	go func() { done <- srv.Start(context.Background()) }()
	<-srv.Ready()
	requestDone := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + srv.netListener.Addr().String())
		if err == nil {
			_, err = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		requestDone <- err
	}()
	<-entered
	stopped := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { stopped <- srv.Shutdown(ctx) }()
	select {
	case err := <-stopped:
		t.Fatalf("shutdown returned before request finished: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-requestDone)
	require.NoError(t, <-stopped)
	require.ErrorIs(t, <-done, http.ErrServerClosed)
}

func TestGRPCServer_WireAndDrain(t *testing.T) {
	srv := NewGRPCServer(&config.Config{GRPCPort: "0", Message: "grpc-test"})
	srv.listenAddr = "127.0.0.1:0"
	done := make(chan error, 1)
	go func() { done <- srv.Start(context.Background()) }()
	<-srv.Ready()
	conn, err := grpc.NewClient(srv.listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := pb.NewEchoServiceClient(conn).Echo(ctx, &pb.EchoRequest{})
	require.NoError(t, err)
	require.Equal(t, "grpc-test", response.Message)
	stream, err := reflection.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&reflection.ServerReflectionRequest{MessageRequest: &reflection.ServerReflectionRequest_ListServices{ListServices: ""}}))
	_, err = stream.Recv()
	require.NoError(t, err)
	stopped := make(chan error, 1)
	go func() { stopped <- srv.Shutdown(ctx) }()
	select {
	case err := <-stopped:
		t.Fatalf("shutdown interrupted an open stream: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	require.NoError(t, stream.CloseSend())
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
	require.NoError(t, <-stopped)
	require.NoError(t, <-done)
}

func TestQUICServer_WireAndDrain(t *testing.T) {
	srv := NewQUICServer(&config.Config{QUICPort: "0"})
	srv.listenAddr = "127.0.0.1:0"
	done := make(chan error, 1)
	entered, release := make(chan struct{}), make(chan struct{})
	srv.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		_ = json.NewEncoder(w).Encode(map[string]string{"protocol": r.Proto})
	})
	go func() { done <- srv.Start(context.Background()) }()
	<-srv.Ready()
	// The server generates a self-signed certificate for this local transport test.
	transport := &http3.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	defer func() { _ = transport.Close() }()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	requestDone := make(chan error, 1)
	go func() {
		resp, err := client.Get("https://" + srv.packet.LocalAddr().String())
		if err == nil {
			if resp.ProtoMajor != 3 {
				err = io.ErrUnexpectedEOF
			}
			_, copyErr := io.Copy(io.Discard, resp.Body)
			if err == nil {
				err = copyErr
			}
			_ = resp.Body.Close()
		}
		requestDone <- err
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- srv.Shutdown(ctx) }()
	select {
	case err := <-stopped:
		t.Fatalf("shutdown interrupted HTTP/3 request: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-requestDone)
	require.NoError(t, transport.Close())
	require.NoError(t, <-stopped)
	require.ErrorIs(t, <-done, http.ErrServerClosed)
}

func TestQUICServer_ConnectionAdmission(t *testing.T) {
	srv := NewQUICServer(&config.Config{})
	srv.connections = make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := srv.connectionContext(ctx, nil)
	require.NoError(t, err)
	_, err = srv.connectionContext(context.Background(), nil)
	require.Error(t, err)
	cancel()
	require.Eventually(t, func() bool { return len(srv.connections) == 0 }, time.Second, time.Millisecond)
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	_, err = srv.connectionContext(ctx, nil)
	require.NoError(t, err)
}
