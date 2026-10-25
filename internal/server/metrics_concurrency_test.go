package server

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/health"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var defaultGathererMu sync.Mutex

func metricsHandlerWithGatherer(gatherer prometheus.Gatherer, timeout time.Duration, checkers ...*health.Checker) http.Handler {
	defaultGathererMu.Lock()
	defer defaultGathererMu.Unlock()
	previous := prometheus.DefaultGatherer
	prometheus.DefaultGatherer = gatherer
	defer func() { prometheus.DefaultGatherer = previous }()
	return metricsHandler(timeout, checkers...)
}

type blockingGatherer struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (g *blockingGatherer) Gather() ([]*dto.MetricFamily, error) {
	g.calls.Add(1)
	g.entered <- struct{}{}
	<-g.release
	return nil, nil
}

func TestMetricsScrapesAreBoundedAndRecover(t *testing.T) {
	for _, withHealth := range []bool{false, true} {
		name := "default gatherer"
		if withHealth {
			name = "with health gauges"
		}
		t.Run(name, func(t *testing.T) {
			gatherer := &blockingGatherer{entered: make(chan struct{}, 6), release: make(chan struct{})}
			var release sync.Once
			defer release.Do(func() { close(gatherer.release) })
			var checkers []*health.Checker
			if withHealth {
				checkers = append(checkers, health.NewChecker(config.ExternalReadinessProbe{}))
			}
			handler := metricsHandlerWithGatherer(gatherer, time.Second, checkers...)
			results := make(chan int, 5)
			for range 5 {
				go func() {
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
					results <- response.Code
				}()
			}
			for range 5 {
				select {
				case <-gatherer.entered:
				case <-time.After(time.Second):
					t.Fatal("five scrapes did not start gathering")
				}
			}
			rejected := httptest.NewRecorder()
			handler.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			assert.Equal(t, http.StatusServiceUnavailable, rejected.Code)
			assert.Contains(t, rejected.Body.String(), "Limit of concurrent requests reached")
			assert.Equal(t, int32(5), gatherer.calls.Load(), "the sixth request must not gather metrics")
			release.Do(func() { close(gatherer.release) })
			for range 5 {
				select {
				case code := <-results:
					assert.Equal(t, http.StatusOK, code)
				case <-time.After(time.Second):
					t.Fatal("scrape did not finish after its collector was released")
				}
			}
			recovered := httptest.NewRecorder()
			handler.ServeHTTP(recovered, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			require.Equal(t, http.StatusOK, recovered.Code)
			assert.Equal(t, int32(6), gatherer.calls.Load())
		})
	}
}
