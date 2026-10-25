package protection

import (
	"fmt"
	"math"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testOptions() Options {
	return Options{
		Enabled: true, RequestRate: 20, PeerRate: 5,
		IngressRate: 256 << 10, EgressRate: 1 << 20, DailyEgress: 10 << 20,
	}
}

func testGuard(t *testing.T, o Options) (*Guard, *time.Time) {
	t.Helper()
	g, err := NewGuard(o)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, g.Close()) })
	now := time.Now()
	g.now = func() time.Time { return now }
	g.operations.updated, g.connections.updated = now, now
	g.ingress.updated, g.egress.updated = now, now
	return g, &now
}

func TestValidateOptions(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*Options)
	}{
		{"zero request rate", func(o *Options) { o.RequestRate = 0 }},
		{"negative request rate", func(o *Options) { o.RequestRate = -1 }},
		{"nan request rate", func(o *Options) { o.RequestRate = math.NaN() }},
		{"infinite request rate", func(o *Options) { o.RequestRate = math.Inf(1) }},
		{"large request rate", func(o *Options) { o.RequestRate = 1e7 }},
		{"negative peer rate", func(o *Options) { o.PeerRate = -1 }},
		{"nan peer rate", func(o *Options) { o.PeerRate = math.NaN() }},
		{"infinite peer rate", func(o *Options) { o.PeerRate = math.Inf(1) }},
		{"zero ingress", func(o *Options) { o.IngressRate = 0 }},
		{"large ingress", func(o *Options) { o.IngressRate = math.MaxInt64 }},
		{"zero egress", func(o *Options) { o.EgressRate = 0 }},
		{"large egress", func(o *Options) { o.EgressRate = math.MaxInt64 }},
		{"zero daily budget", func(o *Options) { o.DailyEgress = 0 }},
		{"negative daily budget", func(o *Options) { o.DailyEgress = -1 }},
		{"blank state path", func(o *Options) { o.StateFile = "  " }},
		{"nul state path", func(o *Options) { o.StateFile = "a\x00b" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := testOptions()
			tt.mutate(&o)
			require.Error(t, ValidateOptions(o))
			_, err := NewGuard(o)
			require.Error(t, err)
		})
	}
	o := testOptions()
	o.PeerRate = 0
	require.NoError(t, ValidateOptions(o))
	require.NoError(t, ValidateOptions(Options{}))
}

func TestDisabledAndClosedGuard(t *testing.T) {
	g, err := NewGuard(Options{})
	require.NoError(t, err)
	require.Nil(t, g)
	require.NoError(t, g.AllowOperation("invalid"))
	require.NoError(t, g.AllowConnection("invalid"))
	require.NoError(t, g.AllowProxyConnection())
	require.NoError(t, g.AllowPeerConnection("invalid"))
	require.NoError(t, g.AllowIngress(100))
	require.NoError(t, g.ReserveEgress(100))
	require.Equal(t, Snapshot{}, g.Snapshot())
	require.NoError(t, g.Close())

	g, _ = testGuard(t, testOptions())
	require.NoError(t, g.Close())
	require.ErrorIs(t, g.AllowOperation("127.0.0.1"), ErrClosed)
	require.ErrorIs(t, g.AllowConnection("127.0.0.1"), ErrClosed)
	require.ErrorIs(t, g.AllowProxyConnection(), ErrClosed)
	require.ErrorIs(t, g.AllowPeerConnection("127.0.0.1"), ErrClosed)
	require.ErrorIs(t, g.AllowIngress(1), ErrClosed)
	require.ErrorIs(t, g.ReserveEgress(1), ErrClosed)
}

func TestProxyAdmissionChargesGlobalOnceAndPeersSeparately(t *testing.T) {
	o := testOptions()
	o.RequestRate, o.PeerRate = 1, 0.1
	g, _ := testGuard(t, o)
	require.NoError(t, g.AllowProxyConnection())
	require.NoError(t, g.AllowPeerConnection("192.0.2.1:1000"))
	require.NoError(t, g.AllowProxyConnection())
	require.ErrorIs(t, g.AllowPeerConnection("192.0.2.1:2000"), ErrPeerRate)
	require.NoError(t, g.AllowProxyConnection())
	require.NoError(t, g.AllowPeerConnection("[2001:db8::1]:1000"))
	require.ErrorIs(t, g.AllowProxyConnection(), ErrConnectionRate)
	require.Len(t, g.peers, 2, "the proxy hop must not get a shared peer budget")
}

func TestOperationAndConnectionBudgets(t *testing.T) {
	o := testOptions()
	o.RequestRate, o.PeerRate = 1, 0
	g, now := testGuard(t, o)
	for range 3 {
		require.NoError(t, g.AllowOperation("192.0.2.1:1234"))
		require.NoError(t, g.AllowConnection("192.0.2.1:1234"))
	}
	for range 3 {
		require.NoError(t, g.AllowOperation("192.0.2.1:1234"))
	}
	require.ErrorIs(t, g.AllowOperation("198.51.100.1:1234"), ErrRequestRate)
	require.ErrorIs(t, g.AllowConnection("198.51.100.1:1234"), ErrConnectionRate)
	*now = now.Add(time.Second)
	require.NoError(t, g.AllowOperation("192.0.2.1:1234"))
	require.NoError(t, g.AllowConnection("192.0.2.1:1234"))
	*now = now.Add(-time.Hour)
	require.ErrorIs(t, g.AllowOperation("198.51.100.1:1234"), ErrRequestRate)
	*now = now.Add(time.Hour)
	require.ErrorIs(t, g.AllowOperation("198.51.100.1:1234"), ErrRequestRate)
	require.Empty(t, g.peers)
}

func TestDefaultBurstAllowsFourWorkbenchPages(t *testing.T) {
	g, now := testGuard(t, testOptions())
	peer := "192.0.2.1:1234"
	// Four cold workbench tabs each fetch at most 13 page/assets/config resources.
	for range 52 {
		require.NoError(t, g.AllowOperation(peer))
	}
	for range 8 {
		require.NoError(t, g.AllowOperation(peer))
	}
	require.ErrorIs(t, g.AllowOperation(peer), ErrPeerRate)
	for range 60 {
		require.NoError(t, g.AllowOperation("198.51.100.1:1234"))
	}
	require.ErrorIs(t, g.AllowOperation("203.0.113.1:1234"), ErrRequestRate)

	// The independent connection bursts remain 30 per peer and 60 globally.
	for range 30 {
		require.NoError(t, g.AllowConnection(peer))
	}
	require.ErrorIs(t, g.AllowConnection(peer), ErrPeerRate)
	for range 30 {
		require.NoError(t, g.AllowConnection("198.51.100.1:1234"))
	}
	require.ErrorIs(t, g.AllowConnection("203.0.113.1:1234"), ErrConnectionRate)
	*now = now.Add(200 * time.Millisecond)
	require.NoError(t, g.AllowOperation(peer), "peer still refills at five operations per second")
	require.ErrorIs(t, g.AllowOperation(peer), ErrPeerRate)
}

func TestSmallRatesKeepOneOperationBurst(t *testing.T) {
	o := testOptions()
	o.RequestRate = 0.001
	g, _ := testGuard(t, o)
	require.NoError(t, g.AllowOperation("192.0.2.1:1234"))
	require.ErrorIs(t, g.AllowOperation("192.0.2.1:1234"), ErrRequestRate)
	o = testOptions()
	o.PeerRate = 0.001
	g, _ = testGuard(t, o)
	require.NoError(t, g.AllowOperation("192.0.2.1:1234"))
	require.ErrorIs(t, g.AllowOperation("192.0.2.1:1234"), ErrPeerRate)
}

func TestPeerGroupingAndIndependentBuckets(t *testing.T) {
	for _, pair := range [][2]string{
		{"192.0.2.1:1234", "[::ffff:192.0.2.1]:5678"},
		{"[2001:db8:1:2::1]:1234", "[2001:db8:1:2::ffff]:5678"},
		{"invalid-a", "invalid-b"},
	} {
		t.Run(pair[0], func(t *testing.T) {
			o := testOptions()
			o.PeerRate = 1
			g, now := testGuard(t, o)
			for range 6 {
				require.NoError(t, g.AllowOperation(pair[0]))
				require.NoError(t, g.AllowConnection(pair[0]))
			}
			for range 6 {
				require.NoError(t, g.AllowOperation(pair[0]))
			}
			require.ErrorIs(t, g.AllowOperation(pair[1]), ErrPeerRate)
			require.ErrorIs(t, g.AllowConnection(pair[1]), ErrPeerRate)
			require.NoError(t, g.AllowOperation("198.51.100.4:1234"))
			*now = now.Add(time.Second)
			require.NoError(t, g.AllowOperation(pair[1]))
			require.Len(t, g.peers, 2)
		})
	}
}

func TestPeerCacheBoundAndExpiry(t *testing.T) {
	o := testOptions()
	o.RequestRate = 1e6
	g, now := testGuard(t, o)
	for i := range maxPeers + 20 {
		addr := netip.AddrFrom4([4]byte{10, 0, byte(i >> 8), byte(i)})
		require.NoError(t, g.AllowOperation(addr.String()))
	}
	require.Len(t, g.peers, maxPeers)
	require.Equal(t, maxPeers, g.order.Len())
	for i := range 60 {
		require.NoError(t, g.AllowOperation(fmt.Sprintf("invalid-%d", i)))
		// Invalid strings all share one limited identity, rather than growing the map.
	}
	require.ErrorIs(t, g.AllowOperation("invalid-more"), ErrPeerRate)
	require.Len(t, g.peers, maxPeers)
	*now = now.Add(peerTTL)
	require.NoError(t, g.AllowOperation("192.0.2.1"))
	require.Len(t, g.peers, 1)
}

func TestByteBudgetsAndDailyRollover(t *testing.T) {
	o := testOptions()
	o.IngressRate, o.EgressRate, o.DailyEgress = 1, 1, 2<<20
	g, now := testGuard(t, o)
	require.ErrorIs(t, g.AllowIngress(-1), ErrIngressRate)
	require.ErrorIs(t, g.ReserveEgress(-1), ErrEgressRate)
	require.NoError(t, g.AllowIngress(minimumBurst))
	require.ErrorIs(t, g.AllowIngress(1), ErrIngressRate)
	require.NoError(t, g.ReserveEgress(minimumBurst))
	require.ErrorIs(t, g.ReserveEgress(1), ErrEgressRate)
	*now = now.Add(time.Second)
	require.NoError(t, g.AllowIngress(1))
	require.NoError(t, g.ReserveEgress(1))
	require.NoError(t, g.ReserveEgress(0))
	require.Equal(t, int64(minimumBurst+1), g.Snapshot().Reserved)

	// A UTC day resets the allowance; local midnight and a backwards clock do not.
	*now = now.Add(24 * time.Hour)
	require.NoError(t, g.ReserveEgress(100))
	require.Equal(t, int64(100), g.Snapshot().Reserved)
	day := g.Snapshot().Day
	*now = now.Add(-24 * time.Hour)
	require.NoError(t, g.ReserveEgress(100))
	require.Equal(t, day, g.Snapshot().Day)
	require.Equal(t, int64(200), g.Snapshot().Reserved)
}

func TestConcurrentDailyBudgetIsExact(t *testing.T) {
	o := testOptions()
	o.EgressRate, o.DailyEgress = 1000, 1000
	g, _ := testGuard(t, o)
	var wg sync.WaitGroup
	var accepted atomic.Int64
	for range 100 {
		wg.Go(func() {
			for range 20 {
				if g.ReserveEgress(1) == nil {
					accepted.Add(1)
				}
			}
		})
	}
	wg.Wait()
	require.Equal(t, int64(1000), accepted.Load())
	require.Equal(t, int64(1000), g.Snapshot().Reserved)
	require.Zero(t, g.Snapshot().Remaining)
	require.ErrorIs(t, g.ReserveEgress(1), ErrDailyEgress)
}

func BenchmarkAllowOperation(b *testing.B) {
	g, err := NewGuard(Options{
		Enabled: true, RequestRate: 1e6, PeerRate: 1e6,
		IngressRate: 1 << 30, EgressRate: 1 << 30, DailyEgress: math.MaxInt64,
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = g.Close() })
	// Measure accepted requests rather than the cheaper exhausted-bucket path.
	g.operations.burst, g.operations.tokens = math.MaxFloat64, math.MaxFloat64
	p := g.peer("[2001:db8::1]:1234", time.Now())
	p.operations.burst, p.operations.tokens = math.MaxFloat64, math.MaxFloat64
	b.ReportAllocs()
	for b.Loop() {
		_ = g.AllowOperation("[2001:db8::1]:1234")
	}
}
