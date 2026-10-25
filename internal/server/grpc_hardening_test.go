package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	pb "github.com/PhilipSchmid/echo-observatory/v2/proto"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
)

func startHardenedGRPCTestServer(t *testing.T, srv *GRPCServer) string {
	t.Helper()
	srv.listenAddr = "127.0.0.1:0"
	done := make(chan error, 1)
	go func() { done <- srv.Start(context.Background()) }()
	select {
	case <-srv.Ready():
	case err := <-done:
		t.Fatalf("gRPC listener failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("gRPC listener did not bind")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
		require.NoError(t, <-done)
	})
	return srv.listener.Addr().String()
}

func grpcTestClient(t *testing.T, address string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	return conn
}

func TestGRPCReceiveSizeUsesConfiguredLimit(t *testing.T) {
	srv := NewGRPCServer(&config.Config{MaxRequestSize: 64}, nil, nil)
	client := pb.NewEchoServiceClient(grpcTestClient(t, startHardenedGRPCTestServer(t, srv)))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, size := range []int{62, 63} {
		req := &pb.EchoRequest{}
		req.ProtoReflect().SetUnknown(protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), make([]byte, size)))
		_, err := client.Echo(ctx, req)
		if size == 62 {
			require.NoError(t, err, "64 encoded bytes must remain within the limit")
		} else {
			require.Equal(t, codes.ResourceExhausted, status.Code(err), "65 encoded bytes must exceed the configured limit")
		}
	}
	require.Eventually(t, func() bool { return len(srv.requests) == 0 }, time.Second, time.Millisecond, "decoding errors must release admission capacity")
	_, err := client.Echo(ctx, &pb.EchoRequest{})
	require.NoError(t, err, "oversized requests must not break subsequent calls")
}

func TestGRPCIdleConnectionsCloseAndReconnect(t *testing.T) {
	srv := NewGRPCServer(&config.Config{MaxRequestSize: 1024}, nil, nil)
	require.Equal(t, 120*time.Second, srv.idleTimeout)
	srv.idleTimeout = 50 * time.Millisecond
	conn := grpcTestClient(t, startHardenedGRPCTestServer(t, srv))
	client := pb.NewEchoServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := client.Echo(ctx, &pb.EchoRequest{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return conn.GetState() == connectivity.Idle }, 2*time.Second, time.Millisecond)
	_, err = client.Echo(ctx, &pb.EchoRequest{})
	require.NoError(t, err, "clients must reconnect after idle transport cleanup")
}

func TestGRPCMetadataIsBounded(t *testing.T) {
	srv := NewGRPCServer(&config.Config{MaxRequestSize: 1024}, nil, nil)
	client := pb.NewEchoServiceClient(grpcTestClient(t, startHardenedGRPCTestServer(t, srv)))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := client.Echo(ctx, &pb.EchoRequest{})
	require.NoError(t, err, "complete the connection settings exchange")
	_, err = client.Echo(metadata.AppendToOutgoingContext(ctx, "x-example", strings.Repeat("a", 32<<10)), &pb.EchoRequest{})
	require.NoError(t, err)
	_, err = client.Echo(metadata.AppendToOutgoingContext(ctx, "x-example", strings.Repeat("a", 64<<10)), &pb.EchoRequest{})
	require.Error(t, err)
	require.Contains(t, status.Convert(err).Message(), "header list")
	_, err = client.Echo(ctx, &pb.EchoRequest{})
	require.NoError(t, err, "oversized metadata must not break subsequent calls")
}

func TestGRPCAdmissionIsSharedAcrossConnectionsAndRPCModes(t *testing.T) {
	srv := NewGRPCServer(&config.Config{MaxRequestSize: 1024}, nil, nil)
	srv.requests = make(chan struct{}, 1)
	address := startHardenedGRPCTestServer(t, srv)
	first, second := grpcTestClient(t, address), grpcTestClient(t, address)
	watchCtx, stopWatch := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopWatch()
	watch, err := healthpb.NewHealthClient(first).Watch(watchCtx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	_, err = watch.Recv()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rejections := metrics.RejectionsTotal.WithLabelValues("gRPC", "request_limit")
	before := testutil.ToFloat64(rejections)
	echoDone := make(chan error, 1)
	go func() {
		_, err := pb.NewEchoServiceClient(second).Echo(ctx, &pb.EchoRequest{})
		echoDone <- err
	}()
	select {
	case err := <-echoDone:
		t.Fatalf("overloaded unary call did not wait for capacity: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	queueCtx, stopQueue := context.WithCancel(ctx)
	queuedWatch, err := healthpb.NewHealthClient(second).Watch(queueCtx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	stopQueue()
	_, err = queuedWatch.Recv()
	require.Equal(t, codes.Canceled, status.Code(err))
	require.Equal(t, 1, len(srv.requests), "cancelled waiters must not acquire the occupied slot")
	require.Equal(t, before, testutil.ToFloat64(rejections), "waiting is backpressure, not rejection")
	stopWatch()
	require.NoError(t, <-echoDone, "releasing a streaming call must resume the queued unary call")
	require.Eventually(t, func() bool { return len(srv.requests) == 0 }, time.Second, time.Millisecond)
	_, err = pb.NewEchoServiceClient(second).Echo(ctx, &pb.EchoRequest{})
	require.NoError(t, err, "completed streams must release global RPC capacity")
}

func TestGRPCAdmissionPrecedesRequestDecoding(t *testing.T) {
	srv := NewGRPCServer(&config.Config{MaxRequestSize: 1024}, nil, nil)
	address := startHardenedGRPCTestServer(t, srv)
	clients := make([]*grpc.ClientConn, 4)
	for index := range clients {
		clients[index] = grpcTestClient(t, address)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stops := make([]context.CancelFunc, 0, maxGRPCRequests)
	defer func() {
		for _, stop := range stops {
			stop()
		}
	}()
	for index := range maxGRPCRequests {
		requestCtx, stop := context.WithCancel(ctx)
		stops = append(stops, stop)
		_, err := clients[index%len(clients)].NewStream(requestCtx, &grpc.StreamDesc{ClientStreams: true}, pb.EchoService_Echo_FullMethodName)
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool { return len(srv.requests) == maxGRPCRequests }, time.Second, time.Millisecond, "streams must acquire capacity even before sending a protobuf message")
	waiting := make(chan error, 1)
	go func() {
		_, err := pb.NewEchoServiceClient(clients[0]).Echo(ctx, &pb.EchoRequest{})
		waiting <- err
	}()
	select {
	case err := <-waiting:
		t.Fatalf("the 101st call did not wait before decoding: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancelCtx, stopWaiting := context.WithCancel(ctx)
	cancelled, err := clients[1].NewStream(cancelCtx, &grpc.StreamDesc{ClientStreams: true}, pb.EchoService_Echo_FullMethodName)
	require.NoError(t, err)
	stopWaiting()
	require.Equal(t, codes.Canceled, status.Code(cancelled.RecvMsg(&pb.EchoResponse{})))
	require.Equal(t, maxGRPCRequests, len(srv.requests))
	err = clients[2].Invoke(ctx, "/malformed", &pb.EchoRequest{}, &pb.EchoResponse{})
	require.Equal(t, codes.Unimplemented, status.Code(err), "malformed method handling must not acquire decoding capacity")
	stops[0]()
	require.NoError(t, <-waiting, "a cancelled incomplete request must resume the queued call")
	require.Eventually(t, func() bool { return len(srv.requests) == maxGRPCRequests-1 }, time.Second, time.Millisecond)
	for _, stop := range stops {
		stop()
	}
	require.Eventually(t, func() bool { return len(srv.requests) == 0 }, time.Second, time.Millisecond)
}

func TestGRPCAdmissionReleasesOnMethodErrors(t *testing.T) {
	srv := NewGRPCServer(&config.Config{MaxRequestSize: 1024}, nil, nil)
	srv.requests = make(chan struct{}, 1)
	conn := grpcTestClient(t, startHardenedGRPCTestServer(t, srv))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, method := range []string{"/malformed", "/unknown.Service/Missing"} {
		err := conn.Invoke(ctx, method, &pb.EchoRequest{}, &pb.EchoResponse{})
		require.Equal(t, codes.Unimplemented, status.Code(err))
		require.Eventually(t, func() bool { return len(srv.requests) == 0 }, time.Second, time.Millisecond)
		_, err = pb.NewEchoServiceClient(conn).Echo(ctx, &pb.EchoRequest{})
		require.NoError(t, err, "method errors must not strand admission capacity")
		require.Eventually(t, func() bool { return len(srv.requests) == 0 }, time.Second, time.Millisecond)
	}
}
