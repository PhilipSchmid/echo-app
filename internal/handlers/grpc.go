package handlers

import (
	"context"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/metrics"
	"github.com/PhilipSchmid/echo-app/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
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
	method, ok := grpc.Method(ctx)
	if !ok {
		method = "unknown"
	}

	// Get peer info for debug logging
	var remoteAddr string
	if p, ok := peer.FromContext(ctx); ok {
		remoteAddr = p.Addr.String()
	}

	// Debug logging
	logrus.Debugf("[gRPC] Incoming request: %s from %s", method, remoteAddr)
	if md, ok := metadata.FromIncomingContext(ctx); ok && logrus.GetLevel() >= logrus.DebugLevel {
		logrus.Debugf("[gRPC] Request metadata: %+v", md)
	}

	defer func() {
		duration := time.Since(start).Seconds()
		metrics.RecordRequest("gRPC", method, "", duration)
		logrus.Debugf("[gRPC] Response sent to %s in %.3fms", remoteAddr, duration*1000)
	}()

	if req == nil {
		metrics.RecordError("gRPC", "nil_request")
		logrus.Debugf("[gRPC] Nil request from %s", remoteAddr)
		return nil, status.Error(codes.InvalidArgument, "request is nil")
	}
	response := buildGRPCResponse(ctx, s.cfg, method)
	return response, nil
}

// buildGRPCResponse constructs the response struct for gRPC
func buildGRPCResponse(ctx context.Context, cfg *config.Config, method string) *proto.EchoResponse {
	remoteAddr := ""
	if p, ok := peer.FromContext(ctx); ok {
		remoteAddr = p.Addr.String()
	}

	base := NewBaseResponse(cfg, "gRPC", remoteAddr)

	return &proto.EchoResponse{
		Timestamp:  base.Timestamp,
		Message:    base.Message,
		Hostname:   base.Hostname,
		Listener:   base.Listener,
		Node:       base.Node,
		SourceIp:   base.SourceIP,
		GrpcMethod: method,
	}
}
