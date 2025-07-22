package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/handlers"
	"github.com/quic-go/quic-go/http3"
	"github.com/sirupsen/logrus"
)

// QUICServer represents a QUIC/HTTP3 server
type QUICServer struct {
	cfg        *config.Config
	server     *http3.Server
	listenAddr string
}

// NewQUICServer creates a new QUIC server
func NewQUICServer(cfg *config.Config) *QUICServer {
	return &QUICServer{
		cfg:        cfg,
		listenAddr: ":" + cfg.QUICPort,
	}
}

// Name returns the server name
func (s *QUICServer) Name() string {
	return "QUIC"
}

// Start starts the QUIC server
func (s *QUICServer) Start(ctx context.Context) error {
	// Get TLS config
	tlsConfig, err := handlers.GetTLSConfig()
	if err != nil {
		return fmt.Errorf("failed to get TLS config: %w", err)
	}

	// Configure TLS for QUIC
	tlsConfig.NextProtos = []string{"h3", "h3-29"}

	// Create HTTP handler
	mux := http.NewServeMux()
	mux.HandleFunc("/", handlers.QUICHandler(s.cfg))

	// Create QUIC server
	s.server = &http3.Server{
		Addr:      s.listenAddr,
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	logrus.Infof("QUIC server listening on %s", s.listenAddr)

	// Start serving in a goroutine to handle context cancellation
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return s.server.Close()
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully shuts down the QUIC server
func (s *QUICServer) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	// QUIC server doesn't have a graceful shutdown method, just close
	if err := s.server.Close(); err != nil {
		return fmt.Errorf("failed to close QUIC server: %w", err)
	}

	return nil
}
