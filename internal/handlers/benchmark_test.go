package handlers

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/proto"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func BenchmarkHTTPHandler(b *testing.B) {
	cfg := &config.Config{
		Message: "benchmark-test",
		Node:    "bench-node",
	}
	handler := HTTPHandler(cfg, "HTTP")

	req := httptest.NewRequest("GET", "/benchmark", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler(w, req)
	}
}

func BenchmarkHTTPHandlerWithHeaders(b *testing.B) {
	cfg := &config.Config{
		Message:      "benchmark-test",
		Node:         "bench-node",
		PrintHeaders: true,
	}
	handler := HTTPHandler(cfg, "HTTP")

	req := httptest.NewRequest("GET", "/benchmark", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("User-Agent", "BenchmarkBot/1.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Custom-Header", "benchmark-value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler(w, req)
	}
}

func BenchmarkNewBaseResponse(b *testing.B) {
	cfg := &config.Config{
		Message: "benchmark-test",
		Node:    "bench-node",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewBaseResponse(cfg, "HTTP", "10.0.0.1:12345")
	}
}

func BenchmarkExtractIP(b *testing.B) {
	addresses := []string{
		"192.168.1.1:8080",
		"[2001:db8::1]:8080",
		"10.0.0.1:12345",
		"localhost:3000",
		"invalid-address",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = extractIP(addresses[i%len(addresses)])
	}
}

func BenchmarkGRPCEcho(b *testing.B) {
	cfg := &config.Config{
		Message: "benchmark-test",
		Node:    "bench-node",
	}
	server := NewEchoServer(cfg)

	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{
		"user-agent": []string{"grpc-benchmark/1.0"},
	})
	ctx = peer.NewContext(ctx, &peer.Peer{
		Addr: &mockAddr{addr: "10.0.0.1:50051"},
	})

	req := &proto.EchoRequest{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = server.Echo(ctx, req)
	}
}

func BenchmarkTCPHandler(b *testing.B) {
	cfg := &config.Config{
		Message: "benchmark-test",
		Node:    "bench-node",
	}

	// Create a mock connection that discards writes
	conn := &benchmarkConn{
		remoteAddr: "10.0.0.1:9090",
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		TCPHandler(ctx, conn, cfg)
	}
}

// benchmarkConn is a mock connection for benchmarking
type benchmarkConn struct {
	remoteAddr string
}

func (c *benchmarkConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (c *benchmarkConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (c *benchmarkConn) Close() error                       { return nil }
func (c *benchmarkConn) LocalAddr() net.Addr                { return nil }
func (c *benchmarkConn) RemoteAddr() net.Addr               { return &mockAddr{addr: c.remoteAddr} }
func (c *benchmarkConn) SetDeadline(t time.Time) error      { return nil }
func (c *benchmarkConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *benchmarkConn) SetWriteDeadline(t time.Time) error { return nil }

func BenchmarkQUICHandler(b *testing.B) {
	cfg := &config.Config{
		Message: "benchmark-test",
		Node:    "bench-node",
	}
	handler := QUICHandler(cfg)

	req := httptest.NewRequest("GET", "/benchmark", nil)
	req.RemoteAddr = "10.0.0.1:4433"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler(w, req)
	}
}

// Benchmark for JSON marshaling which is a common operation
func BenchmarkJSONMarshal(b *testing.B) {
	cfg := &config.Config{
		Message: "benchmark-test",
		Node:    "bench-node",
	}

	response := HTTPResponse{
		BaseResponse: NewBaseResponse(cfg, "HTTP", "10.0.0.1:8080"),
		HTTPEndpoint: "/benchmark",
		HTTPMethod:   "GET",
		HTTPVersion:  "HTTP/1.1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(response)
	}
}

// Parallel benchmark to test concurrent performance
func BenchmarkHTTPHandlerParallel(b *testing.B) {
	cfg := &config.Config{
		Message: "benchmark-test",
		Node:    "bench-node",
	}
	handler := HTTPHandler(cfg, "HTTP")

	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest("GET", "/benchmark", nil)
		req.RemoteAddr = "10.0.0.1:12345"

		for pb.Next() {
			w := httptest.NewRecorder()
			handler(w, req)
		}
	})
}

// Benchmark for hostname caching efficiency
func BenchmarkGetHostname(b *testing.B) {
	// First call initializes the cache
	_ = getHostname()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getHostname()
	}
}
