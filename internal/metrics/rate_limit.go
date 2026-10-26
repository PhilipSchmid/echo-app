package metrics

import (
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/prometheus/client_golang/prometheus"
)

type rateCollector struct {
	guard                               *protection.Guard
	rate, burst, available              *prometheus.Desc
	consumed, rejected, peers, capacity *prometheus.Desc
}

func newRateCollector(guard *protection.Guard) *rateCollector {
	desc := func(name, help string, labels ...string) *prometheus.Desc {
		return prometheus.NewDesc("echo_observatory_"+name, help, labels, nil)
	}
	return &rateCollector{
		guard:     guard,
		rate:      desc("rate_limit_tokens_per_second", "Configured refill rate in operations, connections or bytes per second; peer buckets apply per client group.", "bucket"),
		burst:     desc("rate_limit_burst_tokens", "Configured token-bucket capacity in operations, connections or bytes.", "bucket"),
		available: desc("rate_limit_available_tokens", "Tokens available at collection; peer buckets show the minimum across recently active client groups, not a sum.", "bucket"),
		consumed:  desc("rate_limit_consumed_tokens_total", "Global tokens charged to accepted operations, connections or reserved transport bytes; not completed requests or provider-billed traffic.", "bucket"),
		rejected:  desc("rate_limit_bucket_rejections_total", "Attempts rejected for insufficient global bucket tokens; excludes peer, concurrency, daily quota and state rejections.", "bucket"),
		peers:     desc("rate_limit_tracked_peers", "Client groups seen within the peer TTL; IPv4 addresses or IPv6 /64 groups, with no identity labels."),
		capacity:  desc("rate_limit_peer_capacity", "Maximum retained client groups for peer rate limiting; zero when peer limits are disabled."),
	}
}

func (c *rateCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{c.rate, c.burst, c.available, c.consumed, c.rejected, c.peers, c.capacity} {
		ch <- desc
	}
}

func (c *rateCollector) Collect(ch chan<- prometheus.Metric) {
	buckets, peers, capacity := c.guard.RateSnapshot()
	if len(buckets) == 0 {
		return
	}
	for _, b := range buckets {
		ch <- prometheus.MustNewConstMetric(c.rate, prometheus.GaugeValue, b.Rate, b.Name)
		ch <- prometheus.MustNewConstMetric(c.burst, prometheus.GaugeValue, b.Burst, b.Name)
		ch <- prometheus.MustNewConstMetric(c.available, prometheus.GaugeValue, b.Available, b.Name)
		if !b.Peer {
			ch <- prometheus.MustNewConstMetric(c.consumed, prometheus.CounterValue, b.Consumed, b.Name)
			ch <- prometheus.MustNewConstMetric(c.rejected, prometheus.CounterValue, float64(b.Rejections), b.Name)
		}
	}
	ch <- prometheus.MustNewConstMetric(c.peers, prometheus.GaugeValue, float64(peers))
	ch <- prometheus.MustNewConstMetric(c.capacity, prometheus.GaugeValue, float64(capacity))
}
