package server

import (
	"context"
	"fmt"
	"net"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	apphealth "github.com/PhilipSchmid/echo-observatory/v2/internal/health"
	pb "github.com/PhilipSchmid/echo-observatory/v2/proto"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/netutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	grpchealth "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// GRPCServer represents a gRPC server
type GRPCServer struct {
	lifecycle
	cfg         *config.Config
	server      *grpc.Server
	listener    net.Listener
	listenAddr  string
	checker     *apphealth.Checker
	health      *grpcHealthService
	unsubscribe func()
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(cfg *config.Config, checkers ...*apphealth.Checker) *GRPCServer {
	checker := apphealth.NewChecker(config.ExternalReadinessProbe{})
	if len(checkers) > 0 && checkers[0] != nil {
		checker = checkers[0]
	}
	return &GRPCServer{
		checker:    checker,
		lifecycle:  newLifecycle(),
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
	s.mu.Lock()
	if err := s.checkStart(ctx); err != nil {
		s.mu.Unlock()
		return err
	}
	opts := []grpc.ServerOption{grpc.MaxConcurrentStreams(100)}
	if s.cfg.GRPCTLS {
		tlsConfig, err := handlers.GetTLSConfig(s.cfg)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to listen on %s: %w", s.listenAddr, err)
	}
	listener = netutil.LimitListener(listener, maxTCPConnections)
	s.listener = listener
	s.server = grpc.NewServer(opts...)

	// Register echo service
	echoServer := handlers.NewEchoServer(s.cfg)
	pb.RegisterEchoServiceServer(s.server, echoServer)

	s.health = &grpcHealthService{Server: grpchealth.NewServer(), checker: s.checker, done: make(chan struct{})}
	s.unsubscribe = s.checker.OnReadyChange(func(ready bool) {
		status := healthpb.HealthCheckResponse_NOT_SERVING
		if ready {
			status = healthpb.HealthCheckResponse_SERVING
		}
		s.health.SetServingStatus("", status)
		s.health.SetServingStatus("echo.EchoService", status)
	})
	healthpb.RegisterHealthServer(s.server, s.health)
	// Register reflection service for grpcurl
	reflection.Register(s.server)

	close(s.ready)
	s.mu.Unlock()
	logrus.Infof("gRPC server listening on %s", listener.Addr())
	return serveWithContext(ctx, func() error { return s.server.Serve(listener) }, s.Shutdown)
}

// Shutdown gracefully shuts down the gRPC server
func (s *GRPCServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
	srv := s.server
	listener := s.listener
	health := s.health
	unsubscribe := s.unsubscribe
	s.mu.Unlock()
	if listener != nil {
		defer func() { _ = listener.Close() }()
	}
	if srv == nil {
		return nil
	}
	if health != nil {
		health.Shutdown()
	}
	if unsubscribe != nil {
		unsubscribe()
	}

	// Try graceful stop first
	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		// Force stop if graceful stop times out
		srv.Stop()
		return fmt.Errorf("gRPC server forced shutdown due to timeout")
	}
}
