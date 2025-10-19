package server

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/handlers"
	"github.com/sirupsen/logrus"
)

const (
	// Maximum concurrent HTTP connections (same as TCP)
	maxHTTPConnections = 1000
)

// HTTPServer represents an HTTP server
type HTTPServer struct {
	cfg         *config.Config
	server      *http.Server
	listenAddr  string
	listener    string
	activeConns int32
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

// connectionLimitMiddleware limits concurrent connections
func (s *HTTPServer) connectionLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		currentConns := atomic.LoadInt32(&s.activeConns)
		if currentConns >= maxHTTPConnections {
			logrus.Warnf("[%s] Connection limit reached (%d), rejecting request from %s",
				s.listener, maxHTTPConnections, r.RemoteAddr)
			http.Error(w, "Service Unavailable: Connection limit reached", http.StatusServiceUnavailable)
			return
		}

		atomic.AddInt32(&s.activeConns, 1)
		defer atomic.AddInt32(&s.activeConns, -1)

		next.ServeHTTP(w, r)
	})
}

// Start starts the HTTP server
func (s *HTTPServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handlers.HTTPHandler(s.cfg, s.listener))

	// Apply connection limit middleware
	handler := s.connectionLimitMiddleware(mux)

	s.server = &http.Server{
		Addr:         s.listenAddr,
		Handler:      handler,
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
