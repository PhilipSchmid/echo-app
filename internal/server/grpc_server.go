package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	apphealth "github.com/PhilipSchmid/echo-observatory/v2/internal/health"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	pb "github.com/PhilipSchmid/echo-observatory/v2/proto"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/netutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	grpchealth "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	maxGRPCRequests    = 100
	maxGRPCConnections = 128
	maxGRPCStreams     = 32
)

// GRPCServer represents a gRPC server
type GRPCServer struct {
	lifecycle
	cfg         *config.Config
	tlsConfig   *tls.Config
	server      *grpc.Server
	listener    net.Listener
	listenAddr  string
	checker     *apphealth.Checker
	health      *grpcHealthService
	unsubscribe func()
	requests    chan struct{}
	idleTimeout time.Duration
	maxAge      time.Duration
	maxAgeGrace time.Duration
	protection  *protection.Guard
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(cfg *config.Config, checker *apphealth.Checker, tlsConfig *tls.Config) *GRPCServer {
	if checker == nil {
		checker = apphealth.NewChecker(config.ExternalReadinessProbe{})
	}
	return &GRPCServer{
		checker:     checker,
		lifecycle:   newLifecycle("gRPC"),
		cfg:         cfg,
		tlsConfig:   tlsConfig,
		listenAddr:  ":" + cfg.GRPCPort,
		requests:    make(chan struct{}, maxGRPCRequests),
		idleTimeout: 120 * time.Second,
		maxAge:      2 * time.Minute,
		maxAgeGrace: 5 * time.Second,
	}
}

// Name returns the server name
func (s *GRPCServer) Name() string {
	return "gRPC"
}

// SetProtection configures shared budgets before Start is called.
func (s *GRPCServer) SetProtection(guard *protection.Guard) {
	s.mu.Lock()
	s.protection = guard
	s.mu.Unlock()
}

// Start starts the gRPC server
func (s *GRPCServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if err := s.checkStart(ctx); err != nil {
		s.mu.Unlock()
		return err
	}
	opts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(maxGRPCStreams),
		grpc.MaxHeaderListSize(64 << 10),
		grpc.ChainUnaryInterceptor(metrics.UnaryServerInterceptor(), s.protectUnary),
		grpc.ChainStreamInterceptor(metrics.StreamServerInterceptor(), s.protectStream),
		grpc.StatsHandler(s),
		grpc.KeepaliveParams(keepalive.ServerParameters{MaxConnectionIdle: s.idleTimeout, MaxConnectionAge: s.maxAge, MaxConnectionAgeGrace: s.maxAgeGrace}),
		grpc.UnknownServiceHandler(func(any, grpc.ServerStream) error {
			return status.Error(codes.Unimplemented, "unknown service or method")
		}),
	}
	opts = append(opts, grpc.MaxSendMsgSize(s.responseSizeLimit()))
	if s.cfg.MaxRequestSize > 0 {
		opts = append(opts, grpc.MaxRecvMsgSize(int(min(s.cfg.MaxRequestSize, int64(math.MaxInt)))))
	}
	if s.cfg.GRPCTLS {
		tlsConfig, err := listenerTLSConfig(s.cfg, s.tlsConfig)
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
	wrapped, err := publicTCPListener(trackConnections(listener, "gRPC"), s.cfg, s.protection, "gRPC")
	if err != nil {
		_ = listener.Close()
		s.mu.Unlock()
		return err
	}
	listener = netutil.LimitListener(wrapped, maxGRPCConnections)
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

	s.markReady()
	s.mu.Unlock()
	defer s.markFinished()
	logrus.Infof("gRPC server listening on %s", listener.Addr())
	return serveWithContext(ctx, func() error { return s.server.Serve(listener) }, s.Shutdown)
}

func (s *GRPCServer) allowOperation(ctx context.Context) error {
	if s.protection == nil {
		return nil
	}
	remoteAddr := ""
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		remoteAddr = p.Addr.String()
	}
	if err := s.protection.AllowOperation(remoteAddr); err != nil {
		metrics.RecordRejection("gRPC", err.Error())
		return status.Error(codes.ResourceExhausted, err.Error())
	}
	return nil
}

func (s *GRPCServer) protectUnary(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if err := s.allowOperation(ctx); err != nil {
		return nil, err
	}
	response, err := handler(ctx, req)
	if err != nil {
		return response, err
	}
	if err := s.checkResponseSize(response); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *GRPCServer) protectStream(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := s.allowOperation(stream.Context()); err != nil {
		return err
	}
	return handler(srv, &protectedGRPCStream{ServerStream: stream, server: s, first: true})
}

func (s *GRPCServer) responseSizeLimit() int {
	limit := s.cfg.MaxResponseSize
	if limit <= 0 {
		limit = 1 << 20
	}
	return int(min(limit, int64(math.MaxInt)))
}

func (s *GRPCServer) checkResponseSize(message any) error {
	if response, ok := message.(proto.Message); ok && proto.Size(response) > s.responseSizeLimit() {
		metrics.RecordError("gRPC", "response_too_large")
		return status.Error(codes.ResourceExhausted, "response exceeds configured size limit")
	}
	return nil
}

type protectedGRPCStream struct {
	grpc.ServerStream
	server *GRPCServer
	first  bool
}

func (s *protectedGRPCStream) RecvMsg(message any) error {
	if s.first {
		s.first = false // The RPC admission already charged the first message.
	} else if err := s.server.allowOperation(s.Context()); err != nil {
		return err
	}
	return s.ServerStream.RecvMsg(message)
}

func (s *protectedGRPCStream) SendMsg(message any) error {
	if err := s.server.checkResponseSize(message); err != nil {
		return err
	}
	return s.ServerStream.SendMsg(message)
}

// TagRPC gates decoding on the RPC goroutine while transport reads remain unblocked.
func (s *GRPCServer) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	select {
	case s.requests <- struct{}{}:
		// The transport context also ends on decoding errors and abandoned requests.
		context.AfterFunc(ctx, func() { <-s.requests })
	case <-ctx.Done():
	}
	return ctx
}

func (*GRPCServer) HandleRPC(context.Context, stats.RPCStats)                         {}
func (*GRPCServer) HandleConn(context.Context, stats.ConnStats)                       {}
func (*GRPCServer) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context { return ctx }

// Shutdown gracefully shuts down the gRPC server
func (s *GRPCServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.markStopped()
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
