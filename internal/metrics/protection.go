package metrics

import (
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterProtection exposes this process's shared budgets. Call once at startup.
func RegisterProtection(g *protection.Guard) {
	prometheus.MustRegister(protectionGauges(g)...)
	prometheus.MustRegister(newRateCollector(g))
}

func protectionGauges(g *protection.Guard) []prometheus.Collector {
	boolean := func(value bool) float64 {
		if value {
			return 1
		}
		return 0
	}
	return []prometheus.Collector{
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "echo_observatory_protection_enabled",
			Help: "Whether shared abuse protection is enabled (1) or disabled (0).",
		}, func() float64 { return boolean(g.Snapshot().Enabled) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "echo_observatory_daily_egress_limit_bytes",
			Help: "Configured daily UTC application transport egress allowance in bytes; not a provider billing limit.",
		}, func() float64 { return float64(g.Snapshot().Limit) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "echo_observatory_daily_egress_reserved_bytes",
			Help: "Egress charged to this UTC budget day, including durable reservations not yet sent and unused leases lost on restart.",
		}, func() float64 { return float64(g.Snapshot().Reserved) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "echo_observatory_daily_egress_remaining_bytes",
			Help: "Unreserved daily egress allowance in bytes; excludes any unused bytes in the current durable reservation.",
		}, func() float64 { return float64(g.Snapshot().Remaining) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "echo_observatory_daily_egress_persistent",
			Help: "Whether daily egress reservations survive restarts (1), or apply only to this process lifetime (0).",
		}, func() float64 { return boolean(g.Snapshot().Persistent) }),
	}
}
