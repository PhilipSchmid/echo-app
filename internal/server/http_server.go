package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/ui"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/netutil"
)

const (
	maxHTTPConnections    = 1000
	maxConcurrentRequests = 100
	maxHTTPHeaderBytes    = 64 << 10
)

// HTTPServer represents an HTTP server
type HTTPServer struct {
	lifecycle
	cfg               *config.Config
	tlsConfig         *tls.Config
	certificateSource handlers.CertificateSource
	server            *http.Server
	netListener       net.Listener
	listenAddr        string
	listener          string
	activeRequests    int32
	protection        *protection.Guard
	closeWebSockets   func()
}

// NewHTTPServer creates a new HTTP server
func NewHTTPServer(cfg *config.Config, useTLS bool, tlsConfig *tls.Config, sources ...handlers.CertificateSource) *HTTPServer {
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
		lifecycle:  newLifecycle(listener),
		cfg:        cfg,
		tlsConfig:  tlsConfig,
		listenAddr: ":" + port,
		listener:   listener,
	}
	if len(sources) > 0 {
		s.certificateSource = sources[0]
	}
	mux := http.NewServeMux()
	websocketHandler, closeWebSockets := handlers.NewWebSocketHandler(cfg, func() *protection.Guard { return s.protection })
	s.closeWebSockets = closeWebSockets
	mux.Handle("/ws", websocketHandler)
	if cfg.UI {
		uiHandler := ui.NewHandler(cfg, listener, func() *handlers.ServerCertificate {
			s.mu.Lock()
			source := s.certificateSource
			s.mu.Unlock()
			if source == nil {
				return nil
			}
			return source()
		})
		mux.Handle("/ui", uiHandler)
		mux.Handle("/ui/", uiHandler)
		mux.Handle("/robots.txt", uiHandler)
		mux.Handle("/sitemap.xml", uiHandler)
	}
	mux.HandleFunc("/", handlers.HTTPHandler(cfg, listener))
	var handler http.Handler = mux
	if useTLS && cfg.QUIC && cfg.QUICAdvertisePort != "" {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Alt-Svc", fmt.Sprintf(`h3=":%s"; ma=300`, cfg.QUICAdvertisePort))
			mux.ServeHTTP(w, r)
		})
	}
	s.server = &http.Server{
		Addr:              s.listenAddr,
		Handler:           browserSecurityHeaders(metrics.InstrumentHTTP(listener, protectHTTP(func() *protection.Guard { return s.protection }, requestLimitMiddleware(&s.activeRequests, handler, listener), listener))),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    maxHTTPHeaderBytes,
		HTTP2:             &http.HTTP2Config{MaxConcurrentStreams: maxConcurrentRequests},
		ErrorLog:          httpErrorLog(listener),
	}
	return s
}

// Name returns the server name
func (s *HTTPServer) Name() string {
	return s.listener
}

func requestLimitMiddleware(active *int32, next http.Handler, listeners ...string) http.Handler {
	listener := "HTTP"
	if len(listeners) > 0 {
		listener = listeners[0]
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(active, 1) > maxConcurrentRequests {
			atomic.AddInt32(active, -1)
			metrics.RecordRejection(listener, "request_limit")
			http.Error(w, "Service Unavailable: Concurrent request limit reached", http.StatusServiceUnavailable)
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
		tlsConfig, err := listenerTLSConfig(s.cfg, s.tlsConfig)
		if err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to get TLS config: %w", err)
		}
		if s.certificateSource == nil {
			s.certificateSource = handlers.TrackServerCertificate(tlsConfig)
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
	ln = netutil.LimitListener(protectListener(trackConnections(ln, s.listener), s.protection, s.listener), maxHTTPConnections)
	s.netListener = ln
	s.markReady()
	s.mu.Unlock()
	defer s.markFinished()
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
	s.markStopped()
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

// SetProtection installs the shared guard before Start.
func (s *HTTPServer) SetProtection(guard *protection.Guard) { s.protection = guard }
