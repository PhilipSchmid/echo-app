package handlers

import (
	"context"
	"net"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/peer"
)

func TestEchoServer_Echo(t *testing.T) {
	cfg := &config.Config{
		Message: "Test gRPC",
		Node:    "Test Node",
	}
	server := &EchoServer{cfg: cfg}
	resp, err := server.Echo(context.Background(), &proto.EchoRequest{})
	if err != nil {
		t.Errorf("Echo failed: %v", err)
	}
	if resp.Message != "Test gRPC" {
		t.Errorf("Expected message 'Test gRPC', got '%s'", resp.Message)
	}
}

func TestGRPCReportsIPv6Peer(t *testing.T) {
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("2001:db8::42"), Port: 50000}})
	response := buildGRPCResponse(ctx, &config.Config{}, "/echo.EchoService/Echo")
	assert.Equal(t, "2001:db8::42", response.SourceIp)
	assert.Equal(t, "IPv6", response.IpVersion)
}
