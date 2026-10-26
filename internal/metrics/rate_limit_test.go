package metrics

import (
	"fmt"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateCollectorHasOnlyBoundedAggregateLabels(t *testing.T) {
	g, err := protection.NewGuard(protection.Options{Enabled: true, RequestRate: 1000000, PeerRate: 5, IngressRate: 1000000, EgressRate: 1000000, DailyEgress: 1000000})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, g.Close()) })
	for i := range 1000 {
		require.NoError(t, g.AllowOperation(fmt.Sprintf("192.0.%d.%d", i/256, i%256)))
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(newRateCollector(g))
	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 7)
	for _, family := range families {
		assert.LessOrEqual(t, len(family.Metric), 6)
		for _, sample := range family.Metric {
			for _, label := range sample.Label {
				assert.Equal(t, "bucket", label.GetName())
				assert.Contains(t, []string{"operations", "connections", "ingress", "egress", "peer_operations", "peer_connections"}, label.GetValue())
			}
		}
	}
	registry = prometheus.NewRegistry()
	registry.MustRegister(newRateCollector(nil))
	families, err = registry.Gather()
	require.NoError(t, err)
	assert.Empty(t, families, "disabled protection must not look like exhausted buckets")
}
