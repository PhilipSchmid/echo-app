package handlers

import (
	"context"
	"testing"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/proto"
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
