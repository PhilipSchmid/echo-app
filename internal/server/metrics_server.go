package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// MetricsServer represents a Prometheus metrics server
type MetricsServer struct {
	cfg        *config.Config
	server     *http.Server
	listenAddr string
}

// NewMetricsServer creates a new metrics server
func NewMetricsServer(cfg *config.Config) *MetricsServer {
	return &MetricsServer{
		cfg:        cfg,
		listenAddr: ":" + cfg.MetricsPort,
	}
}

// Name returns the server name
func (s *MetricsServer) Name() string {
	return "Metrics"
}

// Start starts the metrics server
func (s *MetricsServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	// Add health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			logrus.Errorf("Failed to write health response: %v", err)
		}
	})

	// Add readiness endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("Ready")); err != nil {
			logrus.Errorf("Failed to write readiness response: %v", err)
		}
	})

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
