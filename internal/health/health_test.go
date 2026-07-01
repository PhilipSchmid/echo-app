package health

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
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

func readyStatus(checker *Checker) int {
	w := httptest.NewRecorder()
	checker.ReadyHandler(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	_, _ = io.Copy(io.Discard, w.Result().Body)
	return w.Code
}
