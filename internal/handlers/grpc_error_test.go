package handlers

import (
	"context"
	"net"
	"testing"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func TestEchoServer_NilRequest(t *testing.T) {
	cfg := &config.Config{
		Message: "test-message",
		Node:    "test-node",
	}

	server := NewEchoServer(cfg)

	// Test with nil request
	resp, err := server.Echo(context.Background(), nil)

	assert.Nil(t, resp)
	assert.Error(t, err)

	// Check error is properly formatted
	st, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "nil")
}

func TestEchoServer_WithPeerInfo(t *testing.T) {
	cfg := &config.Config{
		Message: "test-message",
		Node:    "test-node",
	}

	server := NewEchoServer(cfg)

	// Create context with peer info
	p := &peer.Peer{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("192.168.1.1"),
			Port: 50051,
		},
	}
	ctx := peer.NewContext(context.Background(), p)

	// Add method to context
	ctx = grpc.NewContextWithServerTransportStream(ctx, &mockServerTransportStream{
		method: "/echo.EchoService/Echo",
	})

	req := &proto.EchoRequest{}
	resp, err := server.Echo(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "192.168.1.1", resp.SourceIp) // extractIP removes the port
	assert.Equal(t, "/echo.EchoService/Echo", resp.GrpcMethod)
}

func TestEchoServer_WithoutPeerInfo(t *testing.T) {
	cfg := &config.Config{
		Message: "test-message",
		Node:    "test-node",
	}

	server := NewEchoServer(cfg)

	// Context without peer info
	ctx := context.Background()

	req := &proto.EchoRequest{}
	resp, err := server.Echo(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Empty(t, resp.SourceIp)
}

func TestEchoServer_WithoutMethodInfo(t *testing.T) {
	cfg := &config.Config{
		Message: "test-message",
		Node:    "test-node",
	}

	server := NewEchoServer(cfg)

	// Context without method info
	ctx := context.Background()

	req := &proto.EchoRequest{}
	resp, err := server.Echo(ctx, req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "unknown", resp.GrpcMethod)
}

// mockServerTransportStream is a mock implementation for testing
type mockServerTransportStream struct {
	method string
}

func (m *mockServerTransportStream) Method() string {
	return m.method
}

func (m *mockServerTransportStream) SetHeader(md metadata.MD) error {
	return nil
}

func (m *mockServerTransportStream) SendHeader(md metadata.MD) error {
	return nil
}

func (m *mockServerTransportStream) SetTrailer(md metadata.MD) error {
	return nil
}

func TestEchoServer_ConcurrentRequests(t *testing.T) {
	cfg := &config.Config{
		Message: "concurrent-test",
		Node:    "test-node",
	}

	server := NewEchoServer(cfg)

	// Run multiple concurrent requests
	numRequests := 100
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			ctx := context.Background()
			req := &proto.EchoRequest{}
			resp, err := server.Echo(ctx, req)

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}
}
