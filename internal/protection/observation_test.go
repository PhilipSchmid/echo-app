package protection

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateSnapshotRefillsWithoutChangingAdmission(t *testing.T) {
	g, now := testGuard(t, testOptions())
	require.NoError(t, g.AllowOperation("192.0.2.1"))
	require.NoError(t, g.AllowConnection("192.0.2.1"))
	require.NoError(t, g.AllowIngress(100))
	require.NoError(t, g.ReserveEgress(200))
	buckets, peers, capacity := g.RateSnapshot()
	require.Len(t, buckets, 6)
	assert.Equal(t, 1, peers)
	assert.Equal(t, maxPeers, capacity)
	assert.Equal(t, float64(1), buckets[0].Consumed)
	assert.Equal(t, float64(1), buckets[1].Consumed)
	assert.Equal(t, float64(100), buckets[2].Consumed)
	assert.Equal(t, float64(200), buckets[3].Consumed)
	assert.Equal(t, buckets[0].Burst-1, buckets[0].Available)
	assert.Equal(t, buckets[4].Burst-1, buckets[4].Available)
	previous := g.operations
	*now = now.Add(time.Second)
	buckets, _, _ = g.RateSnapshot()
	assert.Equal(t, buckets[0].Burst, buckets[0].Available)
	assert.Equal(t, previous, g.operations, "a scrape must not alter token refill or rejection state")
	*now = now.Add(peerTTL)
	_, peers, _ = g.RateSnapshot()
	assert.Zero(t, peers)
	assert.Len(t, g.peers, 1, "scrapes must not retain or evict peer state")
}

func TestRateSnapshotCountsGlobalRejectionsAndHidesDisabledPeers(t *testing.T) {
	o := testOptions()
	o.RequestRate, o.PeerRate = 1, 0
	g, _ := testGuard(t, o)
	for range 6 {
		require.NoError(t, g.AllowOperation("192.0.2.1"))
	}
	require.ErrorIs(t, g.AllowOperation("192.0.2.1"), ErrRequestRate)
	buckets, peers, capacity := g.RateSnapshot()
	require.Len(t, buckets, 4)
	assert.Equal(t, uint64(1), buckets[0].Rejections)
	assert.Equal(t, float64(6), buckets[0].Consumed)
	assert.Zero(t, buckets[0].Available)
	assert.Zero(t, peers)
	assert.Zero(t, capacity)
	var disabled *Guard
	buckets, peers, capacity = disabled.RateSnapshot()
	assert.Empty(t, buckets)
	assert.Zero(t, peers)
	assert.Zero(t, capacity)
}

func TestRateSnapshotConcurrentWithAdmissions(t *testing.T) {
	g, err := NewGuard(testOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, g.Close()) })
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 100 {
				_ = g.AllowOperation("2001:db8::1")
				_ = g.AllowConnection("2001:db8::1")
				buckets, _, _ := g.RateSnapshot()
				for _, b := range buckets {
					assert.GreaterOrEqual(t, b.Available, float64(0))
					assert.LessOrEqual(t, b.Available, b.Burst)
				}
			}
		})
	}
	workers.Wait()
}
