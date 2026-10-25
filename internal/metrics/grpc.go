package metrics

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func observeRPC(method string) func(error, bool) {
	start := time.Now()
	method = methodLabel(method)
	inFlight := InFlightRequests.WithLabelValues("gRPC")
	inFlight.Inc()
	return func(err error, completed bool) {
		inFlight.Dec()
		code := status.Code(err)
		if code > codes.Unauthenticated {
			code = codes.Unknown
		}
		if errors.Is(err, context.Canceled) {
			code = codes.Canceled
		} else if errors.Is(err, context.DeadlineExceeded) {
			code = codes.DeadlineExceeded
		}
		if !completed {
			code = codes.Internal
			RecordError("gRPC", "panic")
		}
		RecordRequest("gRPC", method, "", time.Since(start).Seconds())
		GRPCResponsesTotal.WithLabelValues(method, code.String()).Inc()
	}
}

// UnaryServerInterceptor observes Echo, health and other unary RPC outcomes.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (_ any, err error) {
		finish := observeRPC(info.FullMethod)
		completed := false
		defer func() { finish(err, completed) }()
		response, err := handler(ctx, req)
		completed = true
		return response, err
	}
}

// StreamServerInterceptor measures the full lifetime of streaming RPCs.
func StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		finish := observeRPC(info.FullMethod)
		completed := false
		defer func() { finish(err, completed) }()
		err = handler(srv, stream)
		completed = true
		return err
	}
}
