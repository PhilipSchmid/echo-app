package metrics

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryRPCObservesHealthAndFinalStatus(t *testing.T) {
	for _, tc := range []struct {
		method, label string
		err           error
		code          string
	}{
		{"/echo.EchoService/Echo", "/echo.EchoService/Echo", nil, "OK"},
		{"/grpc.health.v1.Health/Check", "/grpc.health.v1.Health/Check", status.Error(codes.NotFound, "no such service"), "NotFound"},
		{"/private.Arbitrary/Unbounded", "other", status.Error(codes.Unimplemented, "unknown"), "Unimplemented"},
		{"/echo.EchoService/Echo", "/echo.EchoService/Echo", context.Canceled, "Canceled"},
	} {
		counter := GRPCResponsesTotal.WithLabelValues(tc.label, tc.code)
		before := testutil.ToFloat64(counter)
		_, err := UnaryServerInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: tc.method}, func(context.Context, any) (any, error) {
			return nil, tc.err
		})
		assert.Equal(t, tc.err, err)
		assert.Equal(t, before+1, testutil.ToFloat64(counter))
	}
}

type metricTestStream struct{ grpc.ServerStream }

func (metricTestStream) Context() context.Context { return context.Background() }

func TestStreamingRPCObservesFullLifetimeAndCancellation(t *testing.T) {
	method := "/grpc.health.v1.Health/Watch"
	counter := GRPCResponsesTotal.WithLabelValues(method, "Canceled")
	inFlight := InFlightRequests.WithLabelValues("gRPC")
	before, beforeInFlight := testutil.ToFloat64(counter), testutil.ToFloat64(inFlight)
	err := StreamServerInterceptor()(nil, metricTestStream{}, &grpc.StreamServerInfo{FullMethod: method}, func(any, grpc.ServerStream) error {
		assert.Equal(t, beforeInFlight+1, testutil.ToFloat64(inFlight))
		assert.Equal(t, before, testutil.ToFloat64(counter), "stream completion must wait for handler return")
		return context.Canceled
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, before+1, testutil.ToFloat64(counter))
	assert.Equal(t, beforeInFlight, testutil.ToFloat64(inFlight))
}
