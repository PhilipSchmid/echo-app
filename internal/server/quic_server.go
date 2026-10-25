package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/ui"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/sirupsen/logrus"
)

const (
	maxQUICConnections  = 128
	maxQUICStreams      = 32
	maxHTTP3HeaderBytes = 64 << 10
)

type QUICServer struct {
	lifecycle
	cfg                *config.Config
	tlsConfig          *tls.Config
	certificateSource  handlers.CertificateSource
	server             *http3.Server
	listenAddr         string
	packet             net.PacketConn
	transport          *quic.Transport
	listener           *quic.EarlyListener
	connections        chan struct{}
	activeRequests     int32
	protection         *protection.Guard
	streamTimeout      time.Duration
	connectionLifetime time.Duration
}

func NewQUICServer(cfg *config.Config, tlsConfig *tls.Config, sources ...handlers.CertificateSource) *QUICServer {
	s := &QUICServer{lifecycle: newLifecycle("QUIC"), cfg: cfg, tlsConfig: tlsConfig, listenAddr: net.JoinHostPort(cfg.QUICHost, cfg.QUICPort), connections: make(chan struct{}, maxQUICConnections), streamTimeout: 30 * time.Second, connectionLifetime: 2 * time.Minute}
	if len(sources) > 0 {
		s.certificateSource = sources[0]
	}
	mux := http.NewServeMux()
	if cfg.UI {
		uiHandler := ui.NewHandler(cfg, "QUIC", func() *handlers.ServerCertificate {
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
	mux.HandleFunc("/ws", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "WebSocket requires HTTP/1.1", http.StatusUpgradeRequired)
	})
	mux.HandleFunc("/", handlers.QUICHandler(cfg))
	s.server = &http3.Server{Addr: s.listenAddr, Handler: browserSecurityHeaders(metrics.InstrumentHTTP("QUIC", s.streamDeadlineMiddleware(protectHTTP(func() *protection.Guard { return s.protection }, requestLimitMiddleware(&s.activeRequests, mux, "QUIC"), "QUIC")))), IdleTimeout: 30 * time.Second, MaxHeaderBytes: maxHTTP3HeaderBytes, ConnContext: s.limitConnectionLifetime, Logger: slog.New(http3Diagnostics{})}
	return s
}

func (s *QUICServer) Name() string { return "QUIC" }

func (s *QUICServer) limitConnectionLifetime(ctx context.Context, conn *quic.Conn) context.Context {
	if s.protection == nil {
		return ctx
	}
	// Header parsing precedes the handler's stream deadline, so close the transport too.
	timer := time.AfterFunc(s.connectionLifetime, func() {
		_ = conn.CloseWithError(quic.ApplicationErrorCode(http3.ErrCodeNoError), "connection lifetime reached")
	})
	context.AfterFunc(conn.Context(), func() { _ = timer.Stop() })
	return ctx
}

func (s *QUICServer) streamDeadlineMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controller := http.NewResponseController(w)
		deadline := time.Now().Add(s.streamTimeout)
		for _, setDeadline := range []func(time.Time) error{controller.SetReadDeadline, controller.SetWriteDeadline} {
			if err := setDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
				metrics.RecordError("QUIC", "deadline_error")
				http.Error(w, "Failed to set stream deadline", http.StatusInternalServerError)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *QUICServer) connectionContext(ctx context.Context, info *quic.ClientInfo) (context.Context, error) {
	peer := ""
	if info != nil && info.RemoteAddr != nil {
		peer = info.RemoteAddr.String()
	}
	if err := s.protection.AllowConnection(peer); err != nil {
		metrics.RecordRejection("QUIC", err.Error())
		return nil, err
	}
	select {
	case s.connections <- struct{}{}:
		metrics.ConnectionOpened("QUIC")
		context.AfterFunc(ctx, func() {
			<-s.connections
			metrics.ConnectionClosed("QUIC")
		})
		return ctx, nil
	default:
		metrics.RecordRejection("QUIC", "connection_limit")
		return nil, fmt.Errorf("QUIC connection limit reached")
	}
}

func (s *QUICServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if err := s.checkStart(ctx); err != nil {
		s.mu.Unlock()
		return err
	}
	tlsConfig, err := listenerTLSConfig(s.cfg, s.tlsConfig)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to get TLS config: %w", err)
	}
	if s.certificateSource == nil {
		s.certificateSource = handlers.TrackServerCertificate(tlsConfig)
	}
	packet, err := net.ListenPacket("udp", s.listenAddr)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	packet = protectPacketConn(packet.(*net.UDPConn), s.protection)
	transport := &quic.Transport{Conn: packet, ConnContext: s.connectionContext, VerifySourceAddress: func(net.Addr) bool { return s.protection != nil }}
	listener, err := transport.ListenEarly(http3.ConfigureTLSConfig(tlsConfig), &quic.Config{
		MaxIncomingStreams:   maxQUICStreams,
		HandshakeIdleTimeout: 5 * time.Second,
		MaxIdleTimeout:       30 * time.Second,
	})
	if err != nil {
		_ = transport.Close()
		_ = packet.Close()
		s.mu.Unlock()
		return err
	}
	s.packet, s.transport, s.listener = packet, transport, listener
	s.server.TLSConfig = tlsConfig
	s.markReady()
	s.mu.Unlock()
	defer s.markFinished()
	logrus.Infof("QUIC server listening on %s", packet.LocalAddr())
	return serveWithContext(ctx, func() error { return s.server.ServeListener(listener) }, s.Shutdown)
}

func (s *QUICServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.markStopped()
	srv, listener, transport, packet := s.server, s.listener, s.transport, s.packet
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	err := srv.Shutdown(ctx)
	if listener != nil {
		_ = listener.Close()
	}
	if transport != nil {
		_ = transport.Close()
	}
	if packet != nil {
		_ = packet.Close()
	}
	return err
}

// SetProtection installs the shared guard before Start.
func (s *QUICServer) SetProtection(guard *protection.Guard) { s.protection = guard }
