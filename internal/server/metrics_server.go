package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/health"
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
		lifecycle:  newLifecycle(),
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
	mux.Handle("/metrics", metricsHandler(10*time.Second))

	if s.health != nil {
		mux.HandleFunc("/health", s.health.HealthHandler)
		mux.HandleFunc("/ready", s.health.ReadyHandler)
	}

	s.server = &http.Server{
		Addr:         s.listenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ln, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	ln = netutil.LimitListener(ln, maxHTTPConnections)
	close(s.ready)
	s.mu.Unlock()
	logrus.Infof("Metrics server listening on %s", ln.Addr())
	if err := serveWithContext(ctx, func() error { return s.server.Serve(ln) }, s.Shutdown); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("metrics server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the metrics server
func (s *MetricsServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
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

func metricsHandler(timeout time.Duration) http.Handler {
	return http.TimeoutHandler(promhttp.Handler(), timeout, "Metrics collection timeout")
}
