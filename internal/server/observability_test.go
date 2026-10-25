package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/health"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	pb "github.com/PhilipSchmid/echo-observatory/v2/proto"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	reflection "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
)

func TestHTTPServerMetricsCoverUIFailuresAndRequestLimit(t *testing.T) {
	srv := NewHTTPServer(&config.Config{UI: true, MaxRequestSize: 4}, false, nil)
	t.Cleanup(srv.closeWebSockets)
	for _, tc := range []struct {
		path, body, endpoint string
		code                 int
	}{
		{"/", "too large", "/", http.StatusRequestEntityTooLarge},
		{"/ui/", "", "/ui/", http.StatusOK},
	} {
		counter := metrics.RequestsTotal.WithLabelValues("HTTP", http.MethodGet, tc.endpoint)
		before := testutil.ToFloat64(counter)
		response := httptest.NewRecorder()
		srv.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, strings.NewReader(tc.body)))
		require.Equal(t, tc.code, response.Code)
		require.Equal(t, before+1, testutil.ToFloat64(counter), "requests must be counted once, including early failures and UI")
	}
	rejected := metrics.RejectionsTotal.WithLabelValues("HTTP", "request_limit")
	before := testutil.ToFloat64(rejected)
	atomic.StoreInt32(&srv.activeRequests, maxConcurrentRequests)
	response := httptest.NewRecorder()
	srv.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Equal(t, before+1, testutil.ToFloat64(rejected))
}

func TestHealthMetricsFollowInstanceState(t *testing.T) {
	first := health.NewChecker(config.ExternalReadinessProbe{})
	second := health.NewChecker(config.ExternalReadinessProbe{})
	first.SetReady(false, "starting")
	second.SetHealthy(false, "unhealthy")
	for _, tc := range []struct {
		checker *health.Checker
		healthy string
	}{
		{first, "1"}, {second, "0"},
	} {
		response := httptest.NewRecorder()
		metricsHandler(time.Second, tc.checker).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		require.Equal(t, http.StatusOK, response.Code)
		require.Contains(t, response.Body.String(), "echo_observatory_healthy "+tc.healthy+"\n")
		require.Contains(t, response.Body.String(), "echo_observatory_ready 0\n")
		require.NotContains(t, response.Body.String(), "echo_observatory_external_probe_ready ")
	}
	first.SetReady(true, "")
	response := httptest.NewRecorder()
	metricsHandler(time.Second, first).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "echo_observatory_ready 1\n")
	withProbe := health.NewChecker(config.ExternalReadinessProbe{Type: "icmp", Target: "192.0.2.1", Timeout: time.Second})
	response = httptest.NewRecorder()
	metricsHandler(time.Second, withProbe).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "echo_observatory_external_probe_ready 0\n")
}

func TestTransportConnectionMetricsCloseExactlyOnce(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	tracked := trackConnections(listener, "TLS")
	active := metrics.ActiveConnections.WithLabelValues("TLS")
	total := metrics.ConnectionsTotal.WithLabelValues("TLS")
	beforeActive, beforeTotal := testutil.ToFloat64(active), testutil.ToFloat64(total)
	client, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	conn, err := tracked.Accept()
	require.NoError(t, err)
	require.Equal(t, beforeActive+1, testutil.ToFloat64(active))
	require.Equal(t, beforeTotal+1, testutil.ToFloat64(total))
	require.NoError(t, conn.Close())
	_ = conn.Close()
	require.Equal(t, beforeActive, testutil.ToFloat64(active))
	require.Equal(t, beforeTotal+1, testutil.ToFloat64(total))
}

func TestListenerUpFollowsBindAndShutdown(t *testing.T) {
	for name, create := range testServers() {
		t.Run(name, func(t *testing.T) {
			srv := create()
			up := metrics.ListenerUp.WithLabelValues(srv.Name())
			require.Zero(t, testutil.ToFloat64(up))
			done := make(chan error, 1)
			go func() { done <- srv.Start(context.Background()) }()
			select {
			case <-srv.Ready():
			case err := <-done:
				t.Fatalf("listener failed: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("listener did not bind")
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				require.NoError(t, srv.Shutdown(ctx))
				err := <-done
				require.True(t, err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, grpc.ErrServerStopped), "unexpected shutdown result: %v", err)
				require.Zero(t, testutil.ToFloat64(up))
			}()
			require.Equal(t, float64(1), testutil.ToFloat64(up))
			require.Error(t, srv.Start(context.Background()))
			require.Equal(t, float64(1), testutil.ToFloat64(up), "a duplicate start must not mark a live listener down")
		})
	}
}

func TestGRPCMetricsCoverEchoHealthReflectionAndUnknown(t *testing.T) {
	srv := NewGRPCServer(&config.Config{GRPCPort: "0"}, nil, nil)
	srv.listenAddr = "127.0.0.1:0"
	done := make(chan error, 1)
	go func() { done <- srv.Start(context.Background()) }()
	select {
	case <-srv.Ready():
	case err := <-done:
		t.Fatalf("gRPC listener failed: %v", err)
	case <-time.After(time.Second):
		t.Fatal("gRPC listener did not bind")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		require.NoError(t, <-done)
	})
	conn, err := grpc.NewClient(srv.listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, tc := range []struct {
		method, code string
		call         func() error
	}{
		{"/echo.EchoService/Echo", "OK", func() error {
			_, err := pb.NewEchoServiceClient(conn).Echo(ctx, &pb.EchoRequest{})
			return err
		}},
		{"/grpc.health.v1.Health/Check", "OK", func() error {
			_, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
			return err
		}},
		{"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo", "OK", func() error {
			stream, err := reflection.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
			if err != nil {
				return err
			}
			if err := stream.Send(&reflection.ServerReflectionRequest{MessageRequest: &reflection.ServerReflectionRequest_ListServices{ListServices: ""}}); err != nil {
				return err
			}
			if _, err := stream.Recv(); err != nil {
				return err
			}
			if err := stream.CloseSend(); err != nil {
				return err
			}
			_, err = stream.Recv()
			if err == io.EOF {
				return nil
			}
			return err
		}},
		{"other", "Unimplemented", func() error {
			err := conn.Invoke(ctx, "/unbounded.user.input/Secret", &pb.EchoRequest{}, &pb.EchoResponse{})
			if status.Code(err) == codes.Unimplemented {
				return nil
			}
			return err
		}},
	} {
		counter := metrics.GRPCResponsesTotal.WithLabelValues(tc.method, tc.code)
		before := testutil.ToFloat64(counter)
		require.NoError(t, tc.call())
		require.Eventually(t, func() bool { return testutil.ToFloat64(counter) == before+1 }, time.Second, time.Millisecond, tc.method)
	}
}
