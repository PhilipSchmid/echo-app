package protection

import (
	"math"
	"time"
)

// BucketSnapshot uses one of six fixed bucket names, never a client identity.
type BucketSnapshot struct {
	Name                   string
	Rate, Burst, Available float64
	Consumed               float64
	Rejections             uint64
	Peer                   bool
}

func (b bucket) tokensAt(now time.Time) float64 {
	if now.After(b.updated) {
		return math.Min(b.burst, b.tokens+now.Sub(b.updated).Seconds()*b.rate)
	}
	return b.tokens
}

// RateSnapshot observes refill without mutating admission state or retaining peers.
// Peer availability is the minimum among peers seen in the last ten minutes.
func (g *Guard) RateSnapshot() (buckets []BucketSnapshot, trackedPeers int, capacity int) {
	if g == nil {
		return nil, 0, 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	for i, b := range []bucket{g.operations, g.connections, g.ingress, g.egress} {
		buckets = append(buckets, BucketSnapshot{
			Name: []string{"operations", "connections", "ingress", "egress"}[i],
			Rate: b.rate, Burst: b.burst, Available: b.tokensAt(now), Consumed: b.consumed, Rejections: b.rejections,
		})
	}
	if g.opts.PeerRate <= 0 {
		return buckets, 0, 0
	}
	operations := newBucket(g.opts.PeerRate, math.Ceil(g.opts.PeerRate*12), now)
	connections := newBucket(g.opts.PeerRate, math.Ceil(g.opts.PeerRate*6), now)
	for _, e := range g.peers {
		p := e.Value.(*peerBudget)
		if now.Sub(p.seen) >= peerTTL {
			continue
		}
		trackedPeers++
		operations.tokens = math.Min(operations.tokens, p.operations.tokensAt(now))
		connections.tokens = math.Min(connections.tokens, p.connections.tokensAt(now))
	}
	for i, b := range []bucket{operations, connections} {
		buckets = append(buckets, BucketSnapshot{Name: []string{"peer_operations", "peer_connections"}[i], Rate: b.rate, Burst: b.burst, Available: b.tokens, Peer: true})
	}
	return buckets, trackedPeers, maxPeers
}
