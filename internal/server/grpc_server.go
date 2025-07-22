package server

import (
	"context"
	"fmt"
	"net"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/handlers"
	pb "github.com/PhilipSchmid/echo-app/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// GRPCServer represents a gRPC server
type GRPCServer struct {
	cfg        *config.Config
	server     *grpc.Server
	listener   net.Listener
	listenAddr string
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(cfg *config.Config) *GRPCServer {
	return &GRPCServer{
		cfg:        cfg,
		listenAddr: ":" + cfg.GRPCPort,
	}
}

// Name returns the server name
func (s *GRPCServer) Name() string {
	return "gRPC"
}

// Start starts the gRPC server
func (s *GRPCServer) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.listenAddr, err)
	}
	s.listener = listener

	// Create gRPC server with options
	opts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(100),
	}
	s.server = grpc.NewServer(opts...)

	// Register echo service
	echoServer := handlers.NewEchoServer(s.cfg)
	pb.RegisterEchoServiceServer(s.server, echoServer)

	// Register reflection service for grpcurl
	reflection.Register(s.server)

	logrus.Infof("gRPC server listening on %s", s.listenAddr)

	// Start serving in a goroutine to handle context cancellation
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		s.server.GracefulStop()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully shuts down the gRPC server
func (s *GRPCServer) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	// Try graceful stop first
	stopped := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		// Force stop if graceful stop times out
		s.server.Stop()
		return fmt.Errorf("gRPC server forced shutdown due to timeout")
	}
}
