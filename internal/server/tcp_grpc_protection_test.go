package server

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	pb "github.com/PhilipSchmid/echo-observatory/v2/proto"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	reflection "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
)

func protocolTestGuard(t *testing.T, rate float64, daily int64) *protection.Guard {
	t.Helper()
	guard, err := protection.NewGuard(protection.Options{
		Enabled: true, RequestRate: rate, IngressRate: 1 << 20, EgressRate: 1 << 20, DailyEgress: daily,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, guard.Close()) })
	return guard
}

func startProtectedTCPTestServer(t *testing.T, srv *TCPServer) string {
	t.Helper()
	srv.listenAddr = "127.0.0.1:0"
	done := make(chan error, 1)
	go func() { done <- srv.Start(context.Background()) }()
	select {
	case <-srv.Ready():
	case err := <-done:
		t.Fatalf("TCP listener failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("TCP listener did not bind")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
		require.NoError(t, <-done)
	})
	return srv.listener.Addr().String()
}

func TestTCPProtectionRejectsConnectionsRequestsAndTransfer(t *testing.T) {
	for _, reason := range []string{"connection_rate", "request_rate", "daily_egress"} {
		t.Run(reason, func(t *testing.T) {
			daily := int64(1 << 30)
			if reason == "daily_egress" {
				daily = 1
			}
			guard := protocolTestGuard(t, 0.001, daily)
			switch reason {
			case "connection_rate":
				require.NoError(t, guard.AllowConnection("127.0.0.1:1"))
			case "request_rate":
				require.NoError(t, guard.AllowOperation("127.0.0.1:1"))
			}
			rejections := metrics.RejectionsTotal.WithLabelValues("TCP", reason)
			before := testutil.ToFloat64(rejections)
			srv := NewTCPServer(&config.Config{TCPPort: "0", MaxResponseSize: 1024})
			srv.SetProtection(guard)
			conn, err := net.DialTimeout("tcp", startProtectedTCPTestServer(t, srv), time.Second)
			require.NoError(t, err)
			defer func() { require.NoError(t, conn.Close()) }()
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
			data, err := io.ReadAll(conn)
			require.NoError(t, err)
			require.Empty(t, data, "rejected TCP operations must not return an echo")
			require.Equal(t, before+1, testutil.ToFloat64(rejections))
		})
	}
}

func TestTCPProtectionAccountsSuccessfulWrites(t *testing.T) {
	guard := protocolTestGuard(t, 1000, 1<<30)
	srv := NewTCPServer(&config.Config{TCPPort: "0", Message: "protected-echo", MaxResponseSize: 1024})
	srv.SetProtection(guard)
	conn, err := net.DialTimeout("tcp", startProtectedTCPTestServer(t, srv), time.Second)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	data, err := io.ReadAll(conn)
	require.NoError(t, err)
	require.Contains(t, string(data), "protected-echo")
	require.Equal(t, int64(len(data)), guard.Snapshot().Reserved, "outgoing bytes must be charged before writing")
}

func TestGRPCProtectionSharesUnaryAndStreamingRateLimits(t *testing.T) {
	guard := protocolTestGuard(t, 0.001, 1<<30)
	srv := NewGRPCServer(&config.Config{MaxRequestSize: 1024}, nil, nil)
	srv.SetProtection(guard)
	conn := grpcTestClient(t, startHardenedGRPCTestServer(t, srv))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := pb.NewEchoServiceClient(conn).Echo(ctx, &pb.EchoRequest{})
	require.NoError(t, err)
	rejections := metrics.RejectionsTotal.WithLabelValues("gRPC", "request_rate")
	before := testutil.ToFloat64(rejections)
	_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	watch, err := healthpb.NewHealthClient(conn).Watch(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	_, err = watch.Recv()
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Equal(t, before+2, testutil.ToFloat64(rejections))
}

func TestGRPCProtectionLimitsMessagesWithinReflectionStream(t *testing.T) {
	guard := protocolTestGuard(t, 0.001, 1<<30)
	srv := NewGRPCServer(&config.Config{MaxRequestSize: 1024}, nil, nil)
	srv.SetProtection(guard)
	conn := grpcTestClient(t, startHardenedGRPCTestServer(t, srv))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream, err := reflection.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	require.NoError(t, err)
	request := &reflection.ServerReflectionRequest{MessageRequest: &reflection.ServerReflectionRequest_ListServices{ListServices: ""}}
	require.NoError(t, stream.Send(request))
	_, err = stream.Recv()
	require.NoError(t, err, "the first message must use the existing RPC admission")
	if err := stream.Send(request); err != nil {
		require.ErrorIs(t, err, io.EOF)
	}
	_, err = stream.Recv()
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}

type protectionTestStream struct {
	grpc.ServerStream
	ctx      context.Context
	received int
}

func (s *protectionTestStream) Context() context.Context { return s.ctx }
func (s *protectionTestStream) RecvMsg(any) error {
	s.received++
	return nil
}

func TestGRPCStreamingRateLimitPrecedesMessageDecode(t *testing.T) {
	srv := NewGRPCServer(&config.Config{}, nil, nil)
	srv.SetProtection(protocolTestGuard(t, 0.001, 1<<30))
	raw := &protectionTestStream{ctx: context.Background()}
	err := srv.protectStream(nil, raw, &grpc.StreamServerInfo{}, func(_ any, stream grpc.ServerStream) error {
		require.NoError(t, stream.RecvMsg(&pb.EchoRequest{}))
		return stream.RecvMsg(&pb.EchoRequest{})
	})
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Equal(t, 1, raw.received, "a rejected message must not reach the protobuf decoder")
}

func TestGRPCProtectionRejectsConnectionsBeforeRPCs(t *testing.T) {
	guard := protocolTestGuard(t, 0.001, 1<<30)
	require.NoError(t, guard.AllowConnection("127.0.0.1:1"))
	srv := NewGRPCServer(&config.Config{MaxRequestSize: 1024}, nil, nil)
	srv.SetProtection(guard)
	conn := grpcTestClient(t, startHardenedGRPCTestServer(t, srv))
	rejections := metrics.RejectionsTotal.WithLabelValues("gRPC", "connection_rate")
	before := testutil.ToFloat64(rejections)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	_, err := pb.NewEchoServiceClient(conn).Echo(ctx, &pb.EchoRequest{})
	require.Error(t, err)
	require.Greater(t, testutil.ToFloat64(rejections), before)
}

func TestGRPCProtectionUsesSocketPeerInsteadOfForwardedMetadata(t *testing.T) {
	guard, err := protection.NewGuard(protection.Options{
		Enabled: true, RequestRate: 1000, PeerRate: 0.001, IngressRate: 1 << 20, EgressRate: 1 << 20, DailyEgress: 1 << 30,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, guard.Close()) })
	srv := NewGRPCServer(&config.Config{MaxRequestSize: 1024}, nil, nil)
	srv.SetProtection(guard)
	conn := grpcTestClient(t, startHardenedGRPCTestServer(t, srv))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, claimedPeer := range []string{"192.0.2.1", "2001:db8::1"} {
		_, err = pb.NewEchoServiceClient(conn).Echo(metadata.AppendToOutgoingContext(ctx, "x-forwarded-for", claimedPeer), &pb.EchoRequest{})
		if claimedPeer == "192.0.2.1" {
			require.NoError(t, err)
		} else {
			require.Equal(t, codes.ResourceExhausted, status.Code(err))
			require.Equal(t, "peer_rate", status.Convert(err).Message())
		}
	}
}

func TestGRPCSendSizeUsesConfiguredLimit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		limit   int64
		message int
		want    codes.Code
	}{
		{"within limit", 1024, 64, codes.OK},
		{"over limit", 64, 1024, codes.ResourceExhausted},
		{"legacy fallback permits larger response", 0, 65 << 10, codes.OK},
		{"legacy fallback remains bounded", 0, (1 << 20) + 1, codes.ResourceExhausted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := NewGRPCServer(&config.Config{MaxRequestSize: 1024, MaxResponseSize: tc.limit, Message: strings.Repeat("a", tc.message)}, nil, nil)
			client := pb.NewEchoServiceClient(grpcTestClient(t, startHardenedGRPCTestServer(t, srv)))
			okResponses := metrics.GRPCResponsesTotal.WithLabelValues(pb.EchoService_Echo_FullMethodName, "OK")
			limitedResponses := metrics.GRPCResponsesTotal.WithLabelValues(pb.EchoService_Echo_FullMethodName, "ResourceExhausted")
			errors := metrics.ErrorsTotal.WithLabelValues("gRPC", "response_too_large")
			beforeOK, beforeLimited, beforeErrors := testutil.ToFloat64(okResponses), testutil.ToFloat64(limitedResponses), testutil.ToFloat64(errors)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := client.Echo(ctx, &pb.EchoRequest{})
			require.Equal(t, tc.want, status.Code(err))
			if tc.want == codes.ResourceExhausted {
				require.Equal(t, beforeOK, testutil.ToFloat64(okResponses), "an oversized response must not be counted as successful")
				require.Equal(t, beforeLimited+1, testutil.ToFloat64(limitedResponses))
				require.Equal(t, beforeErrors+1, testutil.ToFloat64(errors))
			} else {
				require.Equal(t, beforeOK+1, testutil.ToFloat64(okResponses))
				require.Equal(t, beforeLimited, testutil.ToFloat64(limitedResponses))
				require.Equal(t, beforeErrors, testutil.ToFloat64(errors))
			}
		})
	}
}

func TestGRPCSendSizeCapsStreamingResponses(t *testing.T) {
	srv := NewGRPCServer(&config.Config{MaxRequestSize: 1024, MaxResponseSize: 16}, nil, nil)
	conn := grpcTestClient(t, startHardenedGRPCTestServer(t, srv))
	responses := metrics.GRPCResponsesTotal.WithLabelValues(reflection.ServerReflection_ServerReflectionInfo_FullMethodName, "ResourceExhausted")
	errors := metrics.ErrorsTotal.WithLabelValues("gRPC", "response_too_large")
	beforeResponses, beforeErrors := testutil.ToFloat64(responses), testutil.ToFloat64(errors)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream, err := reflection.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&reflection.ServerReflectionRequest{MessageRequest: &reflection.ServerReflectionRequest_ListServices{ListServices: ""}}))
	_, err = stream.Recv()
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Equal(t, beforeResponses+1, testutil.ToFloat64(responses))
	require.Equal(t, beforeErrors+1, testutil.ToFloat64(errors))
}

func TestGRPCConnectionAgeReleasesIncompleteRPCs(t *testing.T) {
	srv := NewGRPCServer(&config.Config{MaxRequestSize: 1024}, nil, nil)
	require.Equal(t, 2*time.Minute, srv.maxAge)
	require.Equal(t, 5*time.Second, srv.maxAgeGrace)
	srv.maxAge, srv.maxAgeGrace = 200*time.Millisecond, 50*time.Millisecond
	srv.requests = make(chan struct{}, 1)
	conn := grpcTestClient(t, startHardenedGRPCTestServer(t, srv))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	incomplete, err := conn.NewStream(ctx, &grpc.StreamDesc{ClientStreams: true}, pb.EchoService_Echo_FullMethodName)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(srv.requests) == 1 }, 150*time.Millisecond, time.Millisecond)
	err = incomplete.RecvMsg(&pb.EchoResponse{})
	require.Equal(t, codes.Unavailable, status.Code(err), "connection age must close a stalled active RPC before the client deadline")
	require.Eventually(t, func() bool { return len(srv.requests) == 0 }, time.Second, time.Millisecond)
	_, err = pb.NewEchoServiceClient(conn).Echo(ctx, &pb.EchoRequest{})
	require.NoError(t, err, "a fresh transport must reuse released admission capacity")
}
