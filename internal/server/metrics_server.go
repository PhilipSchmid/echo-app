package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/health"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/netutil"
)

// MetricsServer represents a Prometheus metrics server
type MetricsServer struct {
	lifecycle
	cfg        *config.Config
	server     *http.Server
	listenAddr string
	health     *health.Checker
}

// NewMetricsServer creates a new metrics server
func NewMetricsServer(cfg *config.Config, healthChecker *health.Checker) *MetricsServer {
	if healthChecker == nil {
		healthChecker = health.NewChecker(config.ExternalReadinessProbe{})
	}

	return &MetricsServer{
		lifecycle:  newLifecycle("Metrics"),
		cfg:        cfg,
		listenAddr: ":" + cfg.MetricsPort,
		health:     healthChecker,
	}
}

// Name returns the server name
func (s *MetricsServer) Name() string {
	return "Metrics"
}

// Start starts the metrics server
func (s *MetricsServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if err := s.checkStart(ctx); err != nil {
		s.mu.Unlock()
		return err
	}
	mux := http.NewServeMux()
	// Wrap metrics handler with timeout to prevent hung scrapers
	mux.Handle("/metrics", metricsHandler(10*time.Second, s.health))

	if s.health != nil {
		mux.HandleFunc("/health", s.health.HealthHandler)
		mux.HandleFunc("/ready", s.health.ReadyHandler)
	}

	s.server = &http.Server{
		Addr:         s.listenAddr,
		Handler:      metrics.InstrumentHTTP("Metrics", mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
		ErrorLog:     httpErrorLog("Metrics"),
	}

	ln, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	ln = netutil.LimitListener(trackConnections(ln, "Metrics"), maxHTTPConnections)
	s.markReady()
	s.mu.Unlock()
	defer s.markFinished()
	logrus.Infof("Metrics server listening on %s", ln.Addr())
	if err := serveWithContext(ctx, func() error { return s.server.Serve(ln) }, s.Shutdown); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("metrics server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the metrics server
func (s *MetricsServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.markStopped()
	srv := s.server
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	if err := srv.Shutdown(ctx); err != nil {
		_ = srv.Close()
		return err
	}
	return nil
}

func metricsHandler(timeout time.Duration, checkers ...*health.Checker) http.Handler {
	gatherer := prometheus.DefaultGatherer
	if len(checkers) > 0 && checkers[0] != nil {
		checker := checkers[0]
		registry := prometheus.NewRegistry()
		for _, gauge := range []struct {
			name, help string
			value      func() bool
		}{
			{"echo_observatory_healthy", "Whether the application considers the process live (1) or unhealthy (0).", checker.IsHealthy},
			{"echo_observatory_ready", "Whether all configured listeners and readiness dependencies are ready (1) or not (0).", checker.IsReady},
		} {
			registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: gauge.name, Help: gauge.help}, func() float64 {
				if gauge.value() {
					return 1
				}
				return 0
			}))
		}
		if enabled, _ := checker.ExternalProbeStatus(); enabled {
			registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
				Name: "echo_observatory_external_probe_ready",
				Help: "Whether the configured external readiness probe has succeeded (1); pending or failed is 0.",
			}, func() float64 {
				if _, ready := checker.ExternalProbeStatus(); ready {
					return 1
				}
				return 0
			}))
		}
		gatherer = prometheus.Gatherers{gatherer, registry}
	}
	handler := promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{MaxRequestsInFlight: 5}))
	return http.TimeoutHandler(handler, timeout, "Metrics collection timeout")
}
