// Package protection applies shared, bounded resource budgets across listeners.
package protection

import (
	"container/list"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	maxPeers     = 4096
	peerTTL      = 10 * time.Minute
	minimumBurst = 1 << 20
	leaseSize    = 1 << 20
)

// Errors have fixed values suitable for bounded metric labels.
var (
	ErrRequestRate    = errors.New("request_rate")
	ErrPeerRate       = errors.New("peer_rate")
	ErrConnectionRate = errors.New("connection_rate")
	ErrIngressRate    = errors.New("ingress_rate")
	ErrEgressRate     = errors.New("egress_rate")
	ErrDailyEgress    = errors.New("daily_egress")
	ErrState          = errors.New("state_error")
	ErrClosed         = errors.New("closed")
)

type Options struct {
	Enabled     bool
	RequestRate float64
	PeerRate    float64
	IngressRate int64
	EgressRate  int64
	DailyEgress int64
	StateFile   string
}

// ValidateOptions checks configuration without accessing the filesystem.
func ValidateOptions(o Options) error {
	if !o.Enabled {
		return nil
	}
	if math.IsNaN(o.RequestRate) || math.IsInf(o.RequestRate, 0) || o.RequestRate <= 0 || o.RequestRate > 1e6 {
		return fmt.Errorf("request rate must be greater than zero and at most 1000000")
	}
	if math.IsNaN(o.PeerRate) || math.IsInf(o.PeerRate, 0) || o.PeerRate < 0 || o.PeerRate > 1e6 {
		return fmt.Errorf("peer request rate must be between zero and 1000000")
	}
	if o.IngressRate <= 0 || o.IngressRate > 1<<40 || o.EgressRate <= 0 || o.EgressRate > 1<<40 {
		return fmt.Errorf("ingress and egress byte rates must be greater than zero and at most 1099511627776")
	}
	if o.DailyEgress <= 0 {
		return fmt.Errorf("daily egress budget must be greater than zero")
	}
	if o.StateFile != "" && (strings.TrimSpace(o.StateFile) == "" || strings.ContainsRune(o.StateFile, 0)) {
		return fmt.Errorf("transfer state file must be a valid nonempty path")
	}
	return nil
}

type bucket struct {
	rate, burst, tokens float64
	updated             time.Time
	consumed            float64
	rejections          uint64
}

func newBucket(rate, burst float64, now time.Time) bucket {
	return bucket{rate: rate, burst: burst, tokens: burst, updated: now}
}

func (b *bucket) available(n int64, now time.Time) bool {
	if now.After(b.updated) {
		b.tokens = math.Min(b.burst, b.tokens+now.Sub(b.updated).Seconds()*b.rate)
		b.updated = now
	}
	if float64(n) > b.tokens {
		b.rejections++
		return false
	}
	return true
}

func (b *bucket) consume(n float64) {
	b.tokens -= n
	b.consumed += n
}

type peerBudget struct {
	key                     netip.Prefix
	operations, connections bucket
	seen                    time.Time
}

// Guard is safe for concurrent use. One instance must be shared by all listeners.
// Nil guards allow operations, so explicitly disabled protection has no cost.
type Guard struct {
	mu          sync.Mutex
	opts        Options
	now         func() time.Time
	operations  bucket
	connections bucket
	ingress     bucket
	egress      bucket
	peers       map[netip.Prefix]*list.Element
	order       list.List
	state       budgetState
	available   int64
	lock        *os.File
	closed      bool
	stateErr    error
}

// Snapshot reports charged egress, including durable reservations not yet sent.
// Without a state file the daily budget lasts only for this process's lifetime.
type Snapshot struct {
	Enabled    bool
	Day        string
	Limit      int64
	Reserved   int64
	Remaining  int64
	Persistent bool
}

func NewGuard(o Options) (*Guard, error) {
	if err := ValidateOptions(o); err != nil {
		return nil, err
	}
	if !o.Enabled {
		return nil, nil
	}
	now := time.Now()
	g := &Guard{
		opts: o, now: time.Now,
		operations:  newBucket(o.RequestRate, math.Ceil(o.RequestRate*6), now),
		connections: newBucket(o.RequestRate, math.Ceil(o.RequestRate*3), now),
		ingress:     newBucket(float64(o.IngressRate), float64(max(minimumBurst, o.IngressRate*2)), now),
		egress:      newBucket(float64(o.EgressRate), float64(max(minimumBurst, o.EgressRate*2)), now),
		state:       budgetState{Version: 1, Day: now.UTC().Format(time.DateOnly)},
	}
	if o.PeerRate > 0 {
		g.peers = make(map[netip.Prefix]*list.Element)
	}
	if o.StateFile != "" {
		if err := g.openState(); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func peerKey(peer string) netip.Prefix {
	endpoint, err := netip.ParseAddrPort(peer)
	addr := endpoint.Addr()
	if err != nil {
		addr, err = netip.ParseAddr(peer)
		if err != nil {
			// All invalid identities share one bucket; untrusted strings never become keys.
			return netip.Prefix{}
		}
	}
	addr = addr.Unmap().WithZone("")
	bits := 64
	if addr.Is4() {
		bits = 32
	}
	return netip.PrefixFrom(addr, bits).Masked()
}

func (g *Guard) peer(peer string, now time.Time) *peerBudget {
	for e := g.order.Back(); e != nil && now.Sub(e.Value.(*peerBudget).seen) >= peerTTL; e = g.order.Back() {
		delete(g.peers, e.Value.(*peerBudget).key)
		g.order.Remove(e)
	}
	key := peerKey(peer)
	if e := g.peers[key]; e != nil {
		p := e.Value.(*peerBudget)
		p.seen = now
		g.order.MoveToFront(e)
		return p
	}
	if len(g.peers) == maxPeers {
		e := g.order.Back()
		delete(g.peers, e.Value.(*peerBudget).key)
		g.order.Remove(e)
	}
	p := &peerBudget{
		key: key, seen: now,
		operations:  newBucket(g.opts.PeerRate, math.Ceil(g.opts.PeerRate*12), now),
		connections: newBucket(g.opts.PeerRate, math.Ceil(g.opts.PeerRate*6), now),
	}
	g.peers[key] = g.order.PushFront(p)
	return p
}

func (g *Guard) allow(peer string, connection bool) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return ErrClosed
	}
	now := g.now()
	b, rateErr := &g.operations, ErrRequestRate
	if connection {
		b, rateErr = &g.connections, ErrConnectionRate
	}
	if !b.available(1, now) {
		return rateErr
	}
	if g.opts.PeerRate > 0 {
		p := g.peer(peer, now)
		pb := &p.operations
		if connection {
			pb = &p.connections
		}
		if !pb.available(1, now) {
			return ErrPeerRate
		}
		pb.consume(1)
	}
	b.consume(1)
	return nil
}

// AllowOperation limits HTTP requests, RPCs, and WebSocket messages together.
// peer must be the socket peer or a validated PROXY client, never an HTTP header.
func (g *Guard) AllowOperation(peer string) error { return g.allow(peer, false) }

// AllowConnection separately limits new transports before expensive handshakes.
func (g *Guard) AllowConnection(peer string) error { return g.allow(peer, true) }

// AllowProxyConnection bounds raw transports before reading a trusted header.
func (g *Guard) AllowProxyConnection() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return ErrClosed
	}
	if !g.connections.available(1, g.now()) {
		return ErrConnectionRate
	}
	g.connections.consume(1)
	return nil
}

// AllowPeerConnection completes admission without charging the global bucket again.
func (g *Guard) AllowPeerConnection(peer string) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return ErrClosed
	}
	if g.opts.PeerRate <= 0 {
		return nil
	}
	now := g.now()
	b := &g.peer(peer, now).connections
	if !b.available(1, now) {
		return ErrPeerRate
	}
	b.consume(1)
	return nil
}

// AllowIngress charges received transport bytes. Callers must drop or close on rejection.
func (g *Guard) AllowIngress(n int) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return ErrClosed
	}
	if n < 0 || !g.ingress.available(int64(n), g.now()) {
		return ErrIngressRate
	}
	g.ingress.consume(float64(n))
	return nil
}

// ReserveEgress charges bytes BEFORE a transport write. Failed writes are not
// refunded: conservative accounting prevents retries or crashes bypassing limits.
func (g *Guard) ReserveEgress(n int) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return ErrClosed
	}
	if g.stateErr != nil {
		return ErrState
	}
	now := g.now()
	if n < 0 || !g.egress.available(int64(n), now) {
		return ErrEgressRate
	}
	if n == 0 {
		return nil
	}
	state := g.state
	available := g.available
	if day := now.UTC().Format(time.DateOnly); day > state.Day {
		state = budgetState{Version: 1, Day: day}
		available = 0
	}
	amount := int64(n)
	if amount > available {
		remaining := max(int64(0), g.opts.DailyEgress-state.Reserved)
		if amount-available > remaining {
			return ErrDailyEgress
		}
		reservation := amount - available
		if g.opts.StateFile != "" {
			reservation = min(remaining, max(leaseSize, reservation))
		}
		state.Reserved += reservation
		if g.opts.StateFile != "" {
			if err := g.saveState(state); err != nil {
				// A failed directory sync can follow rename; never retry using stale state.
				g.stateErr = err
				return ErrState
			}
		}
		available += reservation
	}
	g.state, g.available = state, available-amount
	g.egress.consume(float64(n))
	return nil
}

func (g *Guard) Snapshot() Snapshot {
	if g == nil {
		return Snapshot{}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return Snapshot{
		Enabled: true, Day: g.state.Day, Limit: g.opts.DailyEgress, Reserved: g.state.Reserved,
		Remaining:  max(int64(0), g.opts.DailyEgress-g.state.Reserved),
		Persistent: g.opts.StateFile != "",
	}
}

// Close releases the persistent state lock. Operations after Close fail closed.
func (g *Guard) Close() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil
	}
	g.closed = true
	if g.lock != nil {
		return g.lock.Close()
	}
	return nil
}
