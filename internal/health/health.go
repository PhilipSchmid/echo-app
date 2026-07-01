package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
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
	mu          sync.RWMutex
	healthy     bool
	ready       bool
	lastError   string
	lastChecked time.Time
	probe       config.ExternalReadinessProbe
	client      *http.Client
}

// NewChecker creates a Checker. Without an external readiness probe the app is
// considered ready as soon as the probe endpoint is reachable.
func NewChecker(probe config.ExternalReadinessProbe) *Checker {
	ready := !probe.Enabled()
	return &Checker{
		healthy: true,
		ready:   ready,
		probe:   probe,
		client:  &http.Client{Timeout: probe.Timeout},
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
	err := c.check(ctx)
	if err != nil {
		logrus.Warnf("external readiness check failed: %v", err)
		c.SetReady(false, err.Error())
		return
	}
	c.SetReady(true, "")
}

func (c *Checker) check(ctx context.Context) error {
	switch strings.ToLower(c.probe.Type) {
	case "http", "https":
		return c.checkHTTP(ctx)
	case "tcp":
		return c.checkTCP(ctx)
	case "ping", "icmp":
		return c.checkPing(ctx)
	default:
		return fmt.Errorf("unsupported external readiness probe type %q", c.probe.Type)
	}
}

func (c *Checker) checkHTTP(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, c.probe.HTTPMethod, c.probe.Target, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != c.probe.HTTPExpectedStatus {
		return fmt.Errorf("expected HTTP status %d from %s, got %d", c.probe.HTTPExpectedStatus, c.probe.Target, resp.StatusCode)
	}
	return nil
}

func (c *Checker) checkTCP(ctx context.Context) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", c.probe.Target)
	if err != nil {
		return err
	}
	return conn.Close()
}

func (c *Checker) checkPing(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", fmt.Sprintf("%d", int(c.probe.Timeout.Seconds())), c.probe.Target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ping %s failed: %w: %s", c.probe.Target, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// SetHealthy updates the liveness state.
func (c *Checker) SetHealthy(healthy bool, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.healthy = healthy
	c.lastError = reason
	c.lastChecked = time.Now()
}

// SetReady updates the readiness state.
func (c *Checker) SetReady(ready bool, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ready = ready
	c.lastError = reason
	c.lastChecked = time.Now()
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
	ready := c.ready && c.healthy
	reason := c.lastError
	c.mu.RUnlock()
	if !ready {
		http.Error(w, StatusNotReady+": "+reason, http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte(StatusReady))
}
