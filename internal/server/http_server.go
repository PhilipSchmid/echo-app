package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/netutil"
)

const (
	// Maximum concurrent HTTP connections (same as TCP)
	maxHTTPConnections = 1000
)

// HTTPServer represents an HTTP server
type HTTPServer struct {
	lifecycle
	cfg             *config.Config
	server          *http.Server
	netListener     net.Listener
	listenAddr      string
	listener        string
	activeConns     int32
	closeWebSockets func()
}

// NewHTTPServer creates a new HTTP server
func NewHTTPServer(cfg *config.Config, useTLS bool) *HTTPServer {
	port := cfg.HTTPPort
	listener := "HTTP"
	switch {
	case useTLS:
		port = cfg.TLSPort
		listener = "TLS"
	case cfg.H2C:
		listener = "H2C"
	}

	s := &HTTPServer{
		lifecycle:  newLifecycle(),
		cfg:        cfg,
		listenAddr: ":" + port,
		listener:   listener,
	}
	mux := http.NewServeMux()
	websocketHandler, closeWebSockets := handlers.NewWebSocketHandler(cfg)
	s.closeWebSockets = closeWebSockets
	mux.Handle("/ws", websocketHandler)
	mux.HandleFunc("/", handlers.HTTPHandler(cfg, listener))
	s.server = &http.Server{
		Addr:         s.listenAddr,
		Handler:      s.connectionLimitMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return s
}

// Name returns the server name
func (s *HTTPServer) Name() string {
	return s.listener
}

// connectionLimitMiddleware limits concurrent connections
func (s *HTTPServer) connectionLimitMiddleware(next http.Handler) http.Handler {
	return requestLimitMiddleware(&s.activeConns, next)
}

func requestLimitMiddleware(active *int32, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(active, 1) > maxHTTPConnections {
			atomic.AddInt32(active, -1)
			http.Error(w, "Service Unavailable: Connection limit reached", http.StatusServiceUnavailable)
			return
		}

		defer atomic.AddInt32(active, -1)

		next.ServeHTTP(w, r)
	})
}

// Start starts the HTTP server
func (s *HTTPServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if err := s.checkStart(ctx); err != nil {
		s.mu.Unlock()
		return err
	}

	if s.listener == "TLS" {
		tlsConfig, err := handlers.GetTLSConfig(s.cfg)
		if err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to get TLS config: %w", err)
		}
		s.server.TLSConfig = tlsConfig
	}

	if s.listener == "H2C" {
		protocols := new(http.Protocols)
		protocols.SetHTTP1(true)
		protocols.SetUnencryptedHTTP2(true)
		s.server.Protocols = protocols
	}

	ln, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	ln = netutil.LimitListener(ln, maxHTTPConnections)
	s.netListener = ln
	close(s.ready)
	s.mu.Unlock()
	logrus.Infof("%s server listening on %s", s.listener, ln.Addr())
	return serveWithContext(ctx, func() error {
		if s.listener == "TLS" {
			return s.server.ServeTLS(ln, "", "")
		}
		return s.server.Serve(ln)
	}, s.Shutdown)
}

// Shutdown gracefully shuts down the HTTP server
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
	srv := s.server
	closeWebSockets := s.closeWebSockets
	s.mu.Unlock()
	if closeWebSockets != nil {
		closeWebSockets()
	}
	if srv == nil {
		return nil
	}
	if err := srv.Shutdown(ctx); err != nil {
		_ = srv.Close()
		return err
	}
	return nil
}
