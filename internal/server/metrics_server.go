package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/health"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// MetricsServer represents a Prometheus metrics server
type MetricsServer struct {
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
	mux := http.NewServeMux()
	// Wrap metrics handler with timeout to prevent hung scrapers
	mux.Handle("/metrics", http.TimeoutHandler(promhttp.Handler(), 10*time.Second, "Metrics collection timeout"))

	if s.health != nil {
		mux.HandleFunc("/health", s.health.HealthHandler)
		mux.HandleFunc("/ready", s.health.ReadyHandler)
	}

	s.server = &http.Server{
		Addr:         s.listenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	logrus.Infof("Metrics server listening on %s", s.listenAddr)

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("metrics server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the metrics server
func (s *MetricsServer) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
