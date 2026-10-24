package metrics

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
