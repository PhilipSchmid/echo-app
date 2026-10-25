package handlers

import (
	"context"
	"net"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/PhilipSchmid/echo-observatory/v2/proto"
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
func (s *EchoServer) Echo(ctx context.Context, req *proto.EchoRequest) (response *proto.EchoResponse, err error) {
	method, ok := grpc.Method(ctx)
	if !ok {
		method = "unknown"
	}

	// Panic recovery to prevent handler crashes
	defer func() {
		if rec := recover(); rec != nil {
			logrus.Error("[gRPC] Recovered from panic")
			metrics.RecordError("gRPC", "panic")
			response = nil
			err = status.Error(codes.Internal, "internal server error")
		}
	}()

	// Reading incoming metadata clones its map, so only do it for opt-in diagnostics.
	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		start := time.Now()
		remoteAddr := ""
		if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
			remoteAddr = p.Addr.String()
		}
		logrus.Debugf("[gRPC] Request: %s from %s", method, remoteAddr)
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			logrus.Debugf("[gRPC] Request metadata: %+v", diagnosticHeaders(md, false))
		}
		defer func() {
			logrus.Debugf("[gRPC] Response sent to %s in %.3fms", remoteAddr, float64(time.Since(start))/float64(time.Millisecond))
		}()
	}

	if req == nil {
		metrics.RecordError("gRPC", "nil_request")
		logrus.Debug("[gRPC] Nil request")
		return nil, status.Error(codes.InvalidArgument, "request is nil")
	}
	response = buildGRPCResponse(ctx, s.cfg, method)
	return response, nil
}

// buildGRPCResponse constructs the response struct for gRPC
func buildGRPCResponse(ctx context.Context, cfg *config.Config, method string) *proto.EchoResponse {
	remoteAddr := ""
	var remote net.Addr
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		remote = p.Addr
		remoteAddr = p.Addr.String()
	}
	base := NewBaseResponse(cfg, "gRPC", remoteAddr)
	setSocketPeer(&base, remote)

	return &proto.EchoResponse{
		Timestamp:           base.Timestamp,
		Message:             base.Message,
		Hostname:            base.Hostname,
		Listener:            base.Listener,
		Node:                base.Node,
		SourceIp:            base.SourceIP,
		IpVersion:           base.IPVersion,
		SocketPeerIp:        base.SocketPeerIP,
		SocketPeerIpVersion: base.SocketPeerIPVersion,
		SocketPeerAddr:      base.SocketPeerAddr,
		GrpcMethod:          method,
	}
}
