package health

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckerDefaultReady(t *testing.T) {
	checker := NewChecker(config.ExternalReadinessProbe{})

	ready := httptest.NewRecorder()
	checker.ReadyHandler(ready, httptest.NewRequest(http.MethodGet, "/ready", nil))
	assert.Equal(t, http.StatusOK, ready.Code)
	assert.Equal(t, StatusReady, ready.Body.String())

	healthy := httptest.NewRecorder()
	checker.HealthHandler(healthy, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusOK, healthy.Code)
	assert.Equal(t, StatusHealthy, healthy.Body.String())
}

func TestExternalProbeCannotOverrideApplicationReadiness(t *testing.T) {
	checker := NewChecker(config.ExternalReadinessProbe{Type: "tcp", Target: "localhost:80"})
	checker.SetReady(false, "starting listeners")
	checker.setExternalReady(true, "")
	assert.False(t, checker.IsReady())
	assert.Equal(t, http.StatusServiceUnavailable, readyStatus(checker))
	checker.SetReady(true, "")
	assert.True(t, checker.IsReady())
	checker.setExternalReady(false, "dependency failed")
	assert.False(t, checker.IsReady())
	checker.SetReady(false, "shutting down")
	checker.setExternalReady(true, "")
	assert.False(t, checker.IsReady())
	checker.SetHealthy(false, "unhealthy")
	checker.SetReady(true, "")
	assert.False(t, checker.IsReady())
}

func TestICMPDNSRespectsDeadlineAndCancellation(t *testing.T) {
	original := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	t.Cleanup(func() { net.DefaultResolver = original })
	started := time.Now()
	err := runICMPProbe(context.Background(), "slow-dns.invalid", 20*time.Millisecond)
	require.Error(t, err)
	assert.Less(t, time.Since(started), time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = runICMPProbe(ctx, "slow-dns.invalid", time.Minute)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestICMPAddressFamilyCompatibility(t *testing.T) {
	v4 := net.IPAddr{IP: net.ParseIP("192.0.2.1")}
	v6 := net.IPAddr{IP: net.ParseIP("fe80::1"), Zone: "eth0"}
	for _, tc := range []struct {
		name      string
		addresses []net.IPAddr
		want      *net.IPAddr
	}{
		{"dual stack prefers IPv4", []net.IPAddr{v6, v4}, &v4},
		{"IPv4 only", []net.IPAddr{v4}, &v4},
		{"IPv6 preserves zone", []net.IPAddr{v6}, &v6},
		{"no addresses", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, preferredICMPAddress(tc.addresses))
		})
	}
}

func TestCheckerReadinessTracksHTTPProbe(t *testing.T) {
	upstreamReady := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-upstreamReady:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer upstream.Close()

	checker := NewChecker(config.ExternalReadinessProbe{
		Type:               "http",
		Target:             upstream.URL,
		Interval:           10 * time.Millisecond,
		Timeout:            time.Second,
		HTTPMethod:         http.MethodGet,
		HTTPExpectedStatus: http.StatusNoContent,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checker.Start(ctx)

	require.Eventually(t, func() bool { return readyStatus(checker) == http.StatusServiceUnavailable }, time.Second, 10*time.Millisecond)
	close(upstreamReady)
	require.Eventually(t, func() bool { return readyStatus(checker) == http.StatusOK }, time.Second, 10*time.Millisecond)
}

func TestCheckerTCPProbe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	checker := NewChecker(config.ExternalReadinessProbe{
		Type:     "tcp",
		Target:   listener.Addr().String(),
		Interval: time.Second,
		Timeout:  time.Second,
	})
	checker.checkOnce(context.Background())
	assert.Equal(t, http.StatusOK, readyStatus(checker))
}

func TestCheckerICMPProbe(t *testing.T) {
	checker := NewChecker(config.ExternalReadinessProbe{
		Type:     "icmp",
		Target:   "192.0.2.1",
		Interval: time.Second,
		Timeout:  250 * time.Millisecond,
	})
	called := false
	checker.icmpProbe = func(ctx context.Context, target string, timeout time.Duration) error {
		called = true
		assert.NoError(t, ctx.Err())
		assert.Equal(t, "192.0.2.1", target)
		assert.Equal(t, 250*time.Millisecond, timeout)
		return nil
	}

	checker.checkOnce(context.Background())

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, readyStatus(checker))
}

func TestCheckerICMPProbeFailure(t *testing.T) {
	checker := NewChecker(config.ExternalReadinessProbe{
		Type:     "icmp",
		Target:   "192.0.2.1",
		Interval: time.Second,
		Timeout:  time.Second,
	})
	checker.icmpProbe = func(context.Context, string, time.Duration) error {
		return errors.New("icmp failed")
	}

	checker.checkOnce(context.Background())

	assert.Equal(t, http.StatusServiceUnavailable, readyStatus(checker))
}

func TestExternalProbeMetricsDescribeActualChecks(t *testing.T) {
	checker := NewChecker(config.ExternalReadinessProbe{Type: "icmp", Target: "192.0.2.1", Timeout: time.Second})
	enabled, ready := checker.ExternalProbeStatus()
	require.True(t, enabled)
	require.False(t, ready, "a pending probe must not be reported as successful")
	success := metrics.ExternalProbeChecksTotal.WithLabelValues("success")
	failure := metrics.ExternalProbeChecksTotal.WithLabelValues("failure")
	beforeSuccess, beforeFailure := testutil.ToFloat64(success), testutil.ToFloat64(failure)
	beforeDuration := &dto.Metric{}
	require.NoError(t, metrics.ExternalProbeDuration.Write(beforeDuration))
	checker.icmpProbe = func(context.Context, string, time.Duration) error { return nil }
	checker.checkOnce(context.Background())
	_, ready = checker.ExternalProbeStatus()
	require.True(t, ready)
	require.Equal(t, beforeSuccess+1, testutil.ToFloat64(success))
	require.Equal(t, beforeFailure, testutil.ToFloat64(failure))
	checker.SetHealthy(false, "process unhealthy")
	require.False(t, checker.IsHealthy())
	require.False(t, checker.IsReady())
	_, ready = checker.ExternalProbeStatus()
	require.True(t, ready, "dependency success must stay distinct from process health")
	checker.icmpProbe = func(context.Context, string, time.Duration) error { return errors.New("dependency unavailable") }
	checker.checkOnce(context.Background())
	_, ready = checker.ExternalProbeStatus()
	require.False(t, ready)
	require.Equal(t, beforeFailure+1, testutil.ToFloat64(failure))
	afterDuration := &dto.Metric{}
	require.NoError(t, metrics.ExternalProbeDuration.Write(afterDuration))
	require.Equal(t, beforeDuration.GetHistogram().GetSampleCount()+2, afterDuration.GetHistogram().GetSampleCount())
}

func TestCheckerLogsExternalProbeStatusChangesOnly(t *testing.T) {
	hook := captureLogEntries(t)
	checker := NewChecker(config.ExternalReadinessProbe{
		Type:     "icmp",
		Target:   "192.0.2.1",
		Interval: time.Second,
		Timeout:  time.Second,
	})
	attempts := 0
	checker.icmpProbe = func(context.Context, string, time.Duration) error {
		attempts++
		if attempts <= 2 {
			return errors.New("icmp failed")
		}
		return nil
	}

	checker.checkOnce(context.Background())
	checker.checkOnce(context.Background())
	checker.checkOnce(context.Background())
	checker.checkOnce(context.Background())

	entries := hook.Entries()
	require.Len(t, entries, 2)
	assert.Equal(t, logrus.WarnLevel, entries[0].Level)
	assert.Equal(t, "external readiness probe is not ready", entries[0].Message)
	assert.Equal(t, "icmp", entries[0].Data["probe_type"])
	assert.Equal(t, "192.0.2.1", entries[0].Data["target"])
	loggedErr, ok := entries[0].Data[logrus.ErrorKey].(error)
	require.True(t, ok)
	assert.EqualError(t, loggedErr, "icmp failed")
	assert.Equal(t, logrus.InfoLevel, entries[1].Level)
	assert.Equal(t, "external readiness probe is ready", entries[1].Message)
	assert.Equal(t, "icmp", entries[1].Data["probe_type"])
	assert.Equal(t, "192.0.2.1", entries[1].Data["target"])
}

type probeTransportFunc func(*http.Request) (*http.Response, error)

func (f probeTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTTPProbeOmitsURLSecretsFromLogsAndReadiness(t *testing.T) {
	const target = "https://probe-user:probe-pass@upstream.example:8443/private-path?token=secret-query#private-fragment"
	for _, tc := range []struct {
		name, reason string
		status       int
		err          error
	}{
		{name: "success", status: http.StatusOK},
		{name: "unexpected status", status: http.StatusServiceUnavailable, reason: "expected HTTP status 200, got 503"},
		{name: "redirect error", err: &url.Error{Op: "Get", URL: target, Err: errors.New("redirect to " + target)}, reason: "HTTP readiness request failed"},
		{name: "canceled", err: context.Canceled, reason: "HTTP readiness request canceled"},
		{name: "timeout", err: context.DeadlineExceeded, reason: "HTTP readiness request timed out"},
		{name: "DNS error", err: &net.DNSError{Name: target, Err: "private-path"}, reason: "HTTP readiness DNS lookup failed"},
		{name: "TLS error", err: &tls.CertificateVerificationError{Err: errors.New(target)}, reason: "HTTP readiness TLS certificate verification failed"},
		{name: "refused", err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, reason: "HTTP readiness connection refused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hook := captureLogEntries(t)
			checker := NewChecker(config.ExternalReadinessProbe{Type: "https", Target: target, HTTPMethod: http.MethodGet, HTTPExpectedStatus: http.StatusOK, Timeout: time.Second})
			checker.client.Transport = probeTransportFunc(func(r *http.Request) (*http.Response, error) {
				assert.Equal(t, "/private-path", r.URL.Path, "diagnostics must not alter the actual probe URL")
				assert.Equal(t, "token=secret-query", r.URL.RawQuery)
				assert.Equal(t, "probe-user", r.URL.User.Username())
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
			})
			checker.checkOnce(context.Background())
			entries := hook.Entries()
			require.Len(t, entries, 1)
			assert.Equal(t, "upstream.example:8443", entries[0].Data["target"])
			response := httptest.NewRecorder()
			checker.ReadyHandler(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
			assert.Equal(t, tc.reason == "", checker.IsReady())
			if tc.reason != "" {
				assert.Equal(t, http.StatusServiceUnavailable, response.Code)
				assert.Contains(t, response.Body.String(), tc.reason)
			}
			output := fmt.Sprint(entries[0].Data) + response.Body.String()
			for _, secret := range []string{"probe-user", "probe-pass", "private-path", "secret-query", "private-fragment"} {
				assert.NotContains(t, output, secret)
			}
		})
	}
}

func readyStatus(checker *Checker) int {
	w := httptest.NewRecorder()
	checker.ReadyHandler(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	_, _ = io.Copy(io.Discard, w.Result().Body)
	return w.Code
}

type testLogHook struct {
	mu      sync.Mutex
	entries []logrus.Entry
}

func (h *testLogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *testLogHook) Fire(entry *logrus.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	data := make(logrus.Fields, len(entry.Data))
	for key, value := range entry.Data {
		data[key] = value
	}
	h.entries = append(h.entries, logrus.Entry{
		Level:   entry.Level,
		Message: entry.Message,
		Data:    data,
	})
	return nil
}

func (h *testLogHook) Entries() []logrus.Entry {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries := make([]logrus.Entry, len(h.entries))
	copy(entries, h.entries)
	return entries
}

func captureLogEntries(t *testing.T) *testLogHook {
	t.Helper()
	logger := logrus.StandardLogger()
	originalHooks := logger.Hooks
	originalOutput := logger.Out
	logger.Hooks = make(logrus.LevelHooks)
	logger.SetOutput(io.Discard)
	hook := &testLogHook{}
	logger.AddHook(hook)
	t.Cleanup(func() {
		logger.Hooks = originalHooks
		logger.SetOutput(originalOutput)
	})
	return hook
}

func TestReadinessObserverTracksDependenciesAndUnsubscribes(t *testing.T) {
	checker := NewChecker(config.ExternalReadinessProbe{Type: "tcp", Target: "localhost:80"})
	var states []bool
	unsubscribe := checker.OnReadyChange(func(ready bool) { states = append(states, ready) })
	checker.setExternalReady(true, "")
	checker.SetReady(false, "draining")
	require.Equal(t, []bool{false, true, false}, states)
	unsubscribe()
	checker.SetReady(true, "")
	checker.setExternalReady(false, "dependency failed")
	require.Equal(t, []bool{false, true, false}, states)
}
