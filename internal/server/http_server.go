package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/handlers"
	"github.com/sirupsen/logrus"
)

// HTTPServer represents an HTTP server
type HTTPServer struct {
	cfg        *config.Config
	server     *http.Server
	listenAddr string
	listener   string
}

// NewHTTPServer creates a new HTTP server
func NewHTTPServer(cfg *config.Config, useTLS bool) *HTTPServer {
	port := cfg.HTTPPort
	listener := "HTTP"
	if useTLS {
		port = cfg.TLSPort
		listener = "TLS"
	}

	return &HTTPServer{
		cfg:        cfg,
		listenAddr: ":" + port,
		listener:   listener,
	}
}

// Name returns the server name
func (s *HTTPServer) Name() string {
	return s.listener
}

// Start starts the HTTP server
func (s *HTTPServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handlers.HTTPHandler(s.cfg, s.listener))

	s.server = &http.Server{
		Addr:         s.listenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	logrus.Infof("%s server listening on %s", s.listener, s.listenAddr)

	if s.listener == "TLS" {
		tlsConfig, err := handlers.GetTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to get TLS config: %w", err)
		}
		s.server.TLSConfig = tlsConfig
		return s.server.ListenAndServeTLS("", "")
	}

	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
