package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransportBytesHaveBoundedLabelsAndCachedHandles(t *testing.T) {
	TransportBytesTotal.Reset()
	t.Cleanup(TransportBytesTotal.Reset)
	registry := prometheus.NewRegistry()
	registry.MustRegister(TransportBytesTotal)
	for i := range 1000 {
		TransportBytes(fmt.Sprintf("untrusted-listener-%d", i), fmt.Sprintf("untrusted-direction-%d", i)).Add(2)
	}
	received := TransportBytes("TLS", "received")
	sent := TransportBytes("QUIC", "sent")
	received.Add(100)
	sent.Add(200)
	assert.Same(t, received, TransportBytes("TLS", "received"))
	assert.Equal(t, float64(100), testutil.ToFloat64(received))
	assert.Equal(t, float64(200), testutil.ToFloat64(sent))
	assert.Equal(t, float64(2000), testutil.ToFloat64(TransportBytes("unknown", "unknown")))
	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)
	require.Len(t, families[0].Metric, 3)
}

func TestProtectionGauges(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			g, err := protection.NewGuard(protection.Options{
				Enabled: enabled, RequestRate: 20, PeerRate: 5,
				IngressRate: 1 << 20, EgressRate: 1 << 20, DailyEgress: 1000,
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, g.Close()) })
			registry := prometheus.NewRegistry()
			registry.MustRegister(protectionGauges(g)...)
			require.NoError(t, g.ReserveEgress(123))
			want := map[string]float64{
				"echo_observatory_protection_enabled":           0,
				"echo_observatory_daily_egress_limit_bytes":     0,
				"echo_observatory_daily_egress_reserved_bytes":  0,
				"echo_observatory_daily_egress_remaining_bytes": 0,
				"echo_observatory_daily_egress_persistent":      0,
			}
			if enabled {
				want["echo_observatory_protection_enabled"] = 1
				want["echo_observatory_daily_egress_limit_bytes"] = 1000
				want["echo_observatory_daily_egress_reserved_bytes"] = 123
				want["echo_observatory_daily_egress_remaining_bytes"] = 877
			}
			families, err := registry.Gather()
			require.NoError(t, err)
			require.Len(t, families, len(want))
			for _, family := range families {
				require.Contains(t, want, family.GetName())
				require.Len(t, family.Metric, 1)
				assert.Empty(t, family.Metric[0].Label, "no date or peer labels")
				assert.Equal(t, want[family.GetName()], family.Metric[0].GetGauge().GetValue())
			}
			if enabled {
				require.NoError(t, g.ReserveEgress(100))
				families, err = registry.Gather()
				require.NoError(t, err)
				for _, family := range families {
					if family.GetName() == "echo_observatory_daily_egress_reserved_bytes" {
						assert.Equal(t, float64(223), family.Metric[0].GetGauge().GetValue(), "gauge reads live state")
					}
				}
			}
		})
	}
}

func TestProtectionRejectionReasonsAreBounded(t *testing.T) {
	RejectionsTotal.Reset()
	ErrorsTotal.Reset()
	t.Cleanup(func() { RejectionsTotal.Reset(); ErrorsTotal.Reset() })
	registry := prometheus.NewRegistry()
	registry.MustRegister(RejectionsTotal, ErrorsTotal)
	reasons := []string{
		"request_limit", "connection_limit", "session_limit",
		protection.ErrRequestRate.Error(), protection.ErrPeerRate.Error(), protection.ErrConnectionRate.Error(),
		protection.ErrIngressRate.Error(), protection.ErrEgressRate.Error(), protection.ErrDailyEgress.Error(),
		protection.ErrState.Error(), protection.ErrClosed.Error(),
	}
	for _, reason := range reasons {
		RecordRejection("HTTP", reason)
		assert.Equal(t, float64(1), testutil.ToFloat64(RejectionsTotal.WithLabelValues("HTTP", reason)))
	}
	for i := range 1000 {
		RecordRejection(fmt.Sprint(i), fmt.Sprint(i))
		RecordError(fmt.Sprint(i), fmt.Sprint(i))
	}
	RecordError("TLS", "response_too_large")
	assert.Equal(t, float64(1), testutil.ToFloat64(ErrorsTotal.WithLabelValues("TLS", "response_too_large")))
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == "echo_observatory_rejections_total" {
			require.Len(t, family.Metric, len(reasons)+1)
		} else {
			require.Len(t, family.Metric, 2)
		}
	}
}

func TestPersistentProtectionGauge(t *testing.T) {
	switch runtime.GOOS {
	case "darwin", "dragonfly", "freebsd", "linux", "netbsd", "openbsd":
	default:
		t.Skip("persistent transfer budgets require advisory locking on this platform")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	g, err := protection.NewGuard(protection.Options{
		Enabled: true, RequestRate: 20, PeerRate: 5,
		IngressRate: 1 << 20, EgressRate: 1 << 20, DailyEgress: 2 << 20,
		StateFile: filepath.Join(dir, "budget.json"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, g.Close()) })
	require.NoError(t, g.ReserveEgress(1))
	registry := prometheus.NewRegistry()
	registry.MustRegister(protectionGauges(g)...)
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		switch family.GetName() {
		case "echo_observatory_daily_egress_persistent":
			assert.Equal(t, float64(1), family.Metric[0].GetGauge().GetValue())
		case "echo_observatory_daily_egress_reserved_bytes", "echo_observatory_daily_egress_remaining_bytes":
			assert.Equal(t, float64(1<<20), family.Metric[0].GetGauge().GetValue(), "durable reservation includes bytes not yet sent")
		}
	}
}
