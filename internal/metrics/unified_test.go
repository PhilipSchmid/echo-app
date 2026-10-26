package metrics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestRandomHTTPPathsAndRPCMethodsHaveBoundedCardinality(t *testing.T) {
	collectors := []*prometheus.CounterVec{RequestsTotal, HTTPResponsesTotal, GRPCResponsesTotal}
	for _, collector := range collectors {
		collector.Reset()
		t.Cleanup(collector.Reset)
	}
	RequestDuration.Reset()
	t.Cleanup(RequestDuration.Reset)
	registry := prometheus.NewRegistry()
	registry.MustRegister(RequestsTotal, RequestDuration, HTTPResponsesTotal, GRPCResponsesTotal)
	handler := InstrumentHTTP("HTTP", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	for i := range 1000 {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, fmt.Sprintf("/untrusted/%d?secret=%d", i, i), nil))
		_, err := UnaryServerInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: fmt.Sprintf("/random%d.Service/Method%d", i, i)}, func(context.Context, any) (any, error) { return nil, nil })
		require.NoError(t, err)
	}
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		assert.LessOrEqual(t, len(family.Metric), 2, family.GetName())
		for _, sample := range family.Metric {
			for _, label := range sample.Label {
				if (label.GetName() == "endpoint" && label.GetValue() != "") || (label.GetName() == "method" && label.GetValue() != "GET") {
					assert.Equal(t, "other", label.GetValue())
				}
			}
		}
	}
}

func TestRequestMethodsHaveBoundedCardinality(t *testing.T) {
	RequestsTotal.Reset()
	RequestDuration.Reset()
	registry := prometheus.NewRegistry()
	registry.MustRegister(RequestsTotal, RequestDuration)
	for i := range 1000 {
		RecordRequest("HTTP", fmt.Sprintf("CUSTOM%d", i), "/", 0.01)
	}
	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 2)
	for _, family := range families {
		require.Len(t, family.Metric, 1, family.GetName())
		for _, label := range family.Metric[0].Label {
			if label.GetName() == "method" {
				assert.Equal(t, "other", label.GetValue())
			}
		}
	}
}

func TestKnownRequestMethodsArePreserved(t *testing.T) {
	RequestsTotal.Reset()
	RequestDuration.Reset()
	registry := prometheus.NewRegistry()
	registry.MustRegister(RequestsTotal)
	methods := []string{"GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH", "connection", "/echo.EchoService/Echo", "unknown"}
	for _, method := range methods {
		RecordRequest("test", method, "", 0.01)
	}
	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)
	require.Len(t, families[0].Metric, len(methods))
	seen := make([]string, 0, len(methods))
	for _, metric := range families[0].Metric {
		for _, label := range metric.Label {
			if label.GetName() == "method" {
				seen = append(seen, label.GetValue())
			}
		}
	}
	assert.ElementsMatch(t, methods, seen)
}

func TestObservatoryMetricNames(t *testing.T) {
	RequestsTotal.Reset()
	RequestDuration.Reset()
	ErrorsTotal.Reset()
	ActiveConnections.Reset()
	t.Cleanup(func() {
		RequestsTotal.Reset()
		RequestDuration.Reset()
		ErrorsTotal.Reset()
		ActiveConnections.Reset()
	})
	registry := prometheus.NewRegistry()
	registry.MustRegister(RequestsTotal, RequestDuration, ErrorsTotal, ActiveConnections)
	RecordRequest("HTTP", "GET", "/", 0.01)
	RecordError("HTTP", "test")
	ConnectionOpened("TCP")
	families, err := registry.Gather()
	require.NoError(t, err)
	names := make([]string, 0, len(families))
	for _, family := range families {
		names = append(names, family.GetName())
	}
	assert.ElementsMatch(t, []string{
		"echo_observatory_requests_total",
		"echo_observatory_request_duration_seconds",
		"echo_observatory_errors_total",
		"echo_observatory_active_connections",
	}, names)
}
