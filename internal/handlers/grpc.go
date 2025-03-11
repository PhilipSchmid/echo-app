package handlers

import (
	"context"
	"os"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/proto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Define Prometheus metrics for gRPC
var (
	grpcRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grpc_requests_total",
			Help: "Total number of gRPC requests",
		},
		[]string{"method"},
	)
	grpcRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "grpc_request_duration_seconds",
			Help:    "Duration of gRPC requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)
	grpcErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grpc_errors_total",
			Help: "Total number of gRPC errors",
		},
		[]string{"method"},
	)
)

// EchoServer implements the gRPC EchoService
type EchoServer struct {
	proto.UnimplementedEchoServiceServer
	cfg *config.Config
}

// NewEchoServer creates a new EchoServer instance
func NewEchoServer(cfg *config.Config) *EchoServer {
	return &EchoServer{cfg: cfg}
}

// Echo handles the Echo request
func (s *EchoServer) Echo(ctx context.Context, req *proto.EchoRequest) (*proto.EchoResponse, error) {
	start := time.Now()
	method, _ := grpc.Method(ctx)
	if req == nil {
		grpcErrorsTotal.WithLabelValues(method).Inc()
		return nil, status.Error(codes.InvalidArgument, "request is nil")
	}
	response := buildGRPCResponse(ctx, s.cfg, method)
	duration := time.Since(start).Seconds()
	grpcRequestsTotal.WithLabelValues(method).Inc()
	grpcRequestDuration.WithLabelValues(method).Observe(duration)
	return response, nil
}

// buildGRPCResponse constructs the response struct for gRPC
func buildGRPCResponse(ctx context.Context, cfg *config.Config, method string) *proto.EchoResponse {
	timestamp := time.Now().Format(time.RFC3339)
	host, _ := os.Hostname()
	clientIP := ""
	if p, ok := peer.FromContext(ctx); ok {
		clientIP = p.Addr.String()
	}

	return &proto.EchoResponse{
		Timestamp:  timestamp,
		Message:    cfg.Message,
		Hostname:   host,
		Listener:   "gRPC",
		Node:       cfg.Node,
		SourceIp:   clientIP,
		GrpcMethod: method,
	}
}
