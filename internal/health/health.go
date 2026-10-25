package health

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	probing "github.com/prometheus-community/pro-bing"
	"github.com/sirupsen/logrus"
)

const (
	StatusHealthy   = "healthy"
	StatusUnhealthy = "unhealthy"
	StatusReady     = "ready"
	StatusNotReady  = "not ready"
)

// Checker maintains cheap, cached health and readiness state for HTTP probes.
type Checker struct {
	mu                   sync.RWMutex
	healthy              bool
	ready                bool
	lastError            string
	lastChecked          time.Time
	externalProbeChecked bool
	externalProbeReady   bool
	externalError        string
	probe                config.ExternalReadinessProbe
	client               *http.Client
	icmpProbe            icmpProbeFunc
	readyObservers       map[uint64]func(bool)
	nextObserver         uint64
}

type icmpProbeFunc func(context.Context, string, time.Duration) error

// NewChecker creates a Checker; the server manager controls application readiness.
func NewChecker(probe config.ExternalReadinessProbe) *Checker {
	return &Checker{
		healthy:            true,
		ready:              true,
		externalProbeReady: !probe.Enabled(),
		probe:              probe,
		client:             &http.Client{Timeout: probe.Timeout},
		icmpProbe:          runICMPProbe,
	}
}

// Start begins the optional external readiness controller. It stores results in
// memory so /ready never performs slow dependency I/O on the request path.
func (c *Checker) Start(ctx context.Context) {
	if !c.probe.Enabled() {
		return
	}
	go c.run(ctx)
}

func (c *Checker) run(ctx context.Context) {
	c.checkOnce(ctx)
	ticker := time.NewTicker(c.probe.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			c.SetReady(false, "shutting down")
			return
		case <-ticker.C:
			c.checkOnce(ctx)
		}
	}
}

func (c *Checker) checkOnce(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, c.probe.Timeout)
	defer cancel()
	start := time.Now()
	err := c.check(ctx)
	metrics.RecordExternalProbe(err == nil, time.Since(start).Seconds())
	if err != nil {
		if c.setExternalReady(false, err.Error()) {
			c.logExternalReadinessChange(false, err)
		}
		return
	}
	if c.setExternalReady(true, "") {
		c.logExternalReadinessChange(true, nil)
	}
}

func (c *Checker) logExternalReadinessChange(ready bool, err error) {
	target := c.probe.Target
	if probeType := strings.ToLower(c.probe.Type); probeType == "http" || probeType == "https" {
		target = "invalid HTTP target"
		if parsed, parseErr := url.Parse(c.probe.Target); parseErr == nil && parsed.Host != "" {
			target = parsed.Host
		}
	}
	entry := logrus.WithFields(logrus.Fields{
		"probe_type": c.probe.Type,
		"target":     target,
	})
	if ready {
		entry.Info("external readiness probe is ready")
		return
	}
	if err != nil {
		entry = entry.WithError(err)
	}
	entry.Warn("external readiness probe is not ready")
}

func (c *Checker) check(ctx context.Context) error {
	switch strings.ToLower(c.probe.Type) {
	case "http", "https":
		return c.checkHTTP(ctx)
	case "tcp":
		return c.checkTCP(ctx)
	case "ping", "icmp":
		return c.checkICMP(ctx)
	default:
		return fmt.Errorf("unsupported external readiness probe type %q", c.probe.Type)
	}
}

func (c *Checker) checkHTTP(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, c.probe.HTTPMethod, c.probe.Target, nil)
	if err != nil {
		return errors.New("invalid HTTP readiness request")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return httpProbeError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != c.probe.HTTPExpectedStatus {
		return fmt.Errorf("expected HTTP status %d, got %d", c.probe.HTTPExpectedStatus, resp.StatusCode)
	}
	return nil
}

func httpProbeError(err error) error {
	// Client and redirect errors can include credentials or complete target URLs.
	var dnsError *net.DNSError
	var tlsError *tls.CertificateVerificationError
	var networkError net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return errors.New("HTTP readiness request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("HTTP readiness request timed out")
	case errors.As(err, &dnsError):
		return errors.New("HTTP readiness DNS lookup failed")
	case errors.As(err, &tlsError):
		return errors.New("HTTP readiness TLS certificate verification failed")
	case errors.Is(err, syscall.ECONNREFUSED):
		return errors.New("HTTP readiness connection refused")
	case errors.As(err, &networkError) && networkError.Timeout():
		return errors.New("HTTP readiness request timed out")
	default:
		return errors.New("HTTP readiness request failed")
	}
}

func (c *Checker) checkTCP(ctx context.Context) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", c.probe.Target)
	if err != nil {
		return err
	}
	return conn.Close()
}

func (c *Checker) checkICMP(ctx context.Context) error {
	return c.icmpProbe(ctx, c.probe.Target, c.probe.Timeout)
}

func runICMPProbe(ctx context.Context, target string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, target)
	if err != nil {
		return fmt.Errorf("resolve ICMP readiness target %s: %w", target, err)
	}
	address := preferredICMPAddress(addresses)
	if address == nil {
		return fmt.Errorf("resolve ICMP readiness target %s: no addresses", target)
	}
	pinger := probing.New(target)
	pinger.SetIPAddr(address)
	pinger.Count = 1
	pinger.Timeout = timeout
	pinger.ResolveTimeout = timeout
	pinger.SetPrivileged(true)

	if err := pinger.RunWithContext(ctx); err != nil {
		return fmt.Errorf("ICMP readiness probe for %s failed: %w", target, err)
	}
	stats := pinger.Statistics()
	if stats.PacketsRecv == 0 {
		return fmt.Errorf("ICMP readiness probe for %s failed: no packets received", target)
	}
	return nil
}

func preferredICMPAddress(addresses []net.IPAddr) *net.IPAddr {
	// Preserve net.ResolveIPAddr("ip", target)'s IPv4 preference.
	for i := range addresses {
		if addresses[i].IP.To4() != nil {
			return &addresses[i]
		}
	}
	if len(addresses) == 0 {
		return nil
	}
	return &addresses[0]
}

// SetHealthy updates the liveness state.
func (c *Checker) SetHealthy(healthy bool, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.healthy = healthy
	defer c.notifyReadyLocked()
	c.lastError = reason
	c.lastChecked = time.Now()
}

// SetReady updates the readiness state.
func (c *Checker) SetReady(ready bool, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ready = ready
	defer c.notifyReadyLocked()
	c.lastError = reason
	c.lastChecked = time.Now()
}

// setExternalReady updates readiness from the external probe and reports
// whether the probe's observed status changed.
func (c *Checker) setExternalReady(ready bool, reason string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	changed := !c.externalProbeChecked || c.externalProbeReady != ready
	c.externalProbeChecked = true
	c.externalProbeReady = ready
	defer c.notifyReadyLocked()
	c.externalError = reason
	c.lastChecked = time.Now()
	return changed
}

// IsReady reports the combined application, liveness and dependency state.
func (c *Checker) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready && c.healthy && c.externalProbeReady
}

// IsHealthy reports process liveness independently of readiness dependencies.
func (c *Checker) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.healthy
}

// ExternalProbeStatus reports cached probe configuration and observed readiness.
func (c *Checker) ExternalProbeStatus() (enabled, ready bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.probe.Enabled(), c.externalProbeChecked && c.externalProbeReady
}

// HealthHandler returns 200 only while the app process considers itself live.
func (c *Checker) HealthHandler(w http.ResponseWriter, _ *http.Request) {
	c.mu.RLock()
	healthy := c.healthy
	reason := c.lastError
	c.mu.RUnlock()
	if !healthy {
		http.Error(w, StatusUnhealthy+": "+reason, http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte(StatusHealthy))
}

// ReadyHandler returns cached readiness without blocking on external checks.
func (c *Checker) ReadyHandler(w http.ResponseWriter, _ *http.Request) {
	c.mu.RLock()
	ready := c.ready && c.healthy && c.externalProbeReady
	reason := c.lastError
	if c.ready && c.healthy && !c.externalProbeReady {
		reason = c.externalError
	}
	c.mu.RUnlock()
	if !ready {
		http.Error(w, StatusNotReady+": "+reason, http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte(StatusReady))
}

// OnReadyChange immediately reports readiness and subsequent changes. Callbacks
// must not call Checker methods; they run while the state is locked.
func (c *Checker) OnReadyChange(fn func(bool)) func() {
	c.mu.Lock()
	if c.readyObservers == nil {
		c.readyObservers = make(map[uint64]func(bool))
	}
	id := c.nextObserver
	c.nextObserver++
	c.readyObservers[id] = fn
	fn(c.ready && c.healthy && c.externalProbeReady)
	c.mu.Unlock()
	return func() { c.mu.Lock(); delete(c.readyObservers, id); c.mu.Unlock() }
}

func (c *Checker) notifyReadyLocked() {
	ready := c.ready && c.healthy && c.externalProbeReady
	for _, fn := range c.readyObservers {
		fn(ready)
	}
}
