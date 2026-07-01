package health

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
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
