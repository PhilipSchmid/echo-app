package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/handlers"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/sirupsen/logrus"
)

type QUICServer struct {
	lifecycle
	cfg            *config.Config
	server         *http3.Server
	listenAddr     string
	packet         net.PacketConn
	transport      *quic.Transport
	listener       *quic.EarlyListener
	connections    chan struct{}
	activeRequests int32
}

func NewQUICServer(cfg *config.Config) *QUICServer {
	s := &QUICServer{lifecycle: newLifecycle(), cfg: cfg, listenAddr: ":" + cfg.QUICPort, connections: make(chan struct{}, maxTCPConnections)}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "WebSocket requires HTTP/1.1", http.StatusUpgradeRequired)
	})
	mux.HandleFunc("/", handlers.QUICHandler(cfg))
	s.server = &http3.Server{Addr: s.listenAddr, Handler: requestLimitMiddleware(&s.activeRequests, mux), IdleTimeout: 30 * time.Second, MaxHeaderBytes: 1 << 20}
	return s
}

func (s *QUICServer) Name() string { return "QUIC" }

func (s *QUICServer) connectionContext(ctx context.Context, _ *quic.ClientInfo) (context.Context, error) {
	select {
	case s.connections <- struct{}{}:
		context.AfterFunc(ctx, func() { <-s.connections })
		return ctx, nil
	default:
		return nil, fmt.Errorf("QUIC connection limit reached")
	}
}

func (s *QUICServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if err := s.checkStart(ctx); err != nil {
		s.mu.Unlock()
		return err
	}
	tlsConfig, err := handlers.GetTLSConfig(s.cfg)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to get TLS config: %w", err)
	}
	packet, err := net.ListenPacket("udp", s.listenAddr)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	transport := &quic.Transport{Conn: packet, ConnContext: s.connectionContext}
	listener, err := transport.ListenEarly(http3.ConfigureTLSConfig(tlsConfig), &quic.Config{
		MaxIncomingStreams:   100,
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
	close(s.ready)
	s.mu.Unlock()
	logrus.Infof("QUIC server listening on %s", packet.LocalAddr())
	return serveWithContext(ctx, func() error { return s.server.ServeListener(listener) }, s.Shutdown)
}

func (s *QUICServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
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
