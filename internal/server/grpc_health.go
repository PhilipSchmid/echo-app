package server

import (
	"sync"

	apphealth "github.com/PhilipSchmid/echo-app/internal/health"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

type grpcHealthService struct {
	*health.Server
	checker *apphealth.Checker
	done    chan struct{}
	once    sync.Once
}

func (s *grpcHealthService) Shutdown() {
	s.Server.Shutdown()
	s.once.Do(func() { close(s.done) })
}

func (s *grpcHealthService) Watch(req *healthpb.HealthCheckRequest, stream healthpb.Health_WatchServer) error {
	if req.Service != "" && req.Service != "echo.EchoService" {
		if err := stream.Send(&healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVICE_UNKNOWN}); err != nil {
			return err
		}
		select {
		case <-s.done:
			return nil
		case <-stream.Context().Done():
			return status.FromContextError(stream.Context().Err()).Err()
		}
	}
	changes := make(chan bool, 1)
	unsubscribe := s.checker.OnReadyChange(func(ready bool) {
		select {
		case <-changes:
		default:
		}
		changes <- ready
	})
	defer unsubscribe()
	last := healthpb.HealthCheckResponse_UNKNOWN
	for {
		select {
		case <-s.done:
			// Terminate health watches so they cannot hold graceful shutdown open.
			return stream.Send(&healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_NOT_SERVING})
		case <-stream.Context().Done():
			return status.FromContextError(stream.Context().Err()).Err()
		case ready := <-changes:
			status := healthpb.HealthCheckResponse_NOT_SERVING
			if ready {
				status = healthpb.HealthCheckResponse_SERVING
			}
			if status == last {
				continue
			}
			if err := stream.Send(&healthpb.HealthCheckResponse{Status: status}); err != nil {
				return err
			}
			last = status
		}
	}
}
