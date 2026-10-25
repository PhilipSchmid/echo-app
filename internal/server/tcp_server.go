package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/sirupsen/logrus"
)

const (
	// Maximum concurrent TCP connections
	maxTCPConnections = 1000
	// TCP connection timeout
	tcpTimeout = 30 * time.Second
)

// TCPServer represents a TCP server with connection management
type TCPServer struct {
	lifecycle
	cfg          *config.Config
	listener     net.Listener
	listenAddr   string
	connections  sync.Map
	activeConns  int32
	shuttingDown int32 // Atomic flag to prevent new connections during shutdown
	shutdownOnce sync.Once
	shutdown     chan struct{}
	wg           sync.WaitGroup
	ctx          context.Context
	protection   *protection.Guard
}

// NewTCPServer creates a new TCP server
func NewTCPServer(cfg *config.Config) *TCPServer {
	return &TCPServer{
		lifecycle:  newLifecycle("TCP"),
		cfg:        cfg,
		listenAddr: ":" + cfg.TCPPort,
		shutdown:   make(chan struct{}),
	}
}

// Name returns the server name
func (s *TCPServer) Name() string {
	return "TCP"
}

// SetProtection configures shared budgets before Start is called.
func (s *TCPServer) SetProtection(guard *protection.Guard) {
	s.mu.Lock()
	s.protection = guard
	s.mu.Unlock()
}

// Start starts the TCP server
func (s *TCPServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if err := s.checkStart(ctx); err != nil {
		s.mu.Unlock()
		return err
	}
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to listen on %s: %w", s.listenAddr, err)
	}

	// Store context and listener with mutex protection
	s.ctx = ctx
	s.listener = listener
	guard := s.protection
	s.markReady()
	s.mu.Unlock()
	defer s.markFinished()
	defer func() { _ = listener.Close() }()

	logrus.Infof("TCP server listening on %s", s.listenAddr)

	// Accept connections
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.shutdown:
			return nil
		default:
			// Set accept deadline to check for shutdown periodically
			if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(1 * time.Second)); err != nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-s.shutdown:
					return nil
				default:
					return fmt.Errorf("set TCP accept deadline: %w", err)
				}
			}

			conn, err := listener.Accept()
			if err != nil {
				// Check if it's a timeout (expected) or real error
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				// Check if we're shutting down
				select {
				case <-s.shutdown:
					return nil
				default:
					return fmt.Errorf("accept TCP connection: %w", err)
				}
			}
			if err := guard.AllowConnection(conn.RemoteAddr().String()); err != nil {
				metrics.RecordRejection("TCP", err.Error())
				_ = conn.Close()
				time.Sleep(time.Millisecond)
				continue
			}
			conn = protectConn(conn, guard, "TCP")

			// Use mutex to make shutdown check and wg.Add atomic
			// This prevents race between wg.Add() here and wg.Wait() in Shutdown
			s.mu.Lock()
			if atomic.LoadInt32(&s.shuttingDown) == 1 {
				s.mu.Unlock()
				if err := conn.Close(); err != nil {
					logrus.Debug("TCP connection cleanup failed during shutdown")
				}
				return nil
			}

			// Check connection limit
			currentConns := atomic.LoadInt32(&s.activeConns)
			if currentConns >= maxTCPConnections {
				s.mu.Unlock()
				metrics.RecordRejection("TCP", "connection_limit")
				if err := conn.Close(); err != nil {
					logrus.Debug("Rejected TCP connection cleanup failed")
				}
				time.Sleep(time.Millisecond)
				continue
			}

			// Handle connection - wg.Add protected by mutex
			s.connections.Store(conn.RemoteAddr().String(), conn)
			s.wg.Add(1)
			atomic.AddInt32(&s.activeConns, 1)
			s.mu.Unlock()
			go s.handleConnection(conn)
		}
	}
}

// handleConnection handles a single TCP connection
func (s *TCPServer) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer atomic.AddInt32(&s.activeConns, -1)

	// Store connection for graceful shutdown
	connID := conn.RemoteAddr().String()
	defer s.connections.Delete(connID)

	// Set connection timeout
	if err := conn.SetDeadline(time.Now().Add(tcpTimeout)); err != nil {
		logrus.Debug("TCP connection deadline could not be set")
		_ = conn.Close()
		return
	}

	// Get context safely
	s.mu.Lock()
	ctx := s.ctx
	guard := s.protection
	s.mu.Unlock()

	if err := guard.AllowOperation(conn.RemoteAddr().String()); err != nil {
		metrics.RecordRejection("TCP", err.Error())
		_ = conn.Close()
		return
	}

	// Handle the connection with context
	handlers.TCPHandler(ctx, conn, s.cfg)
}

// Shutdown gracefully shuts down the TCP server
func (s *TCPServer) Shutdown(ctx context.Context) error {
	var err error

	s.shutdownOnce.Do(func() {
		// Set shutdown flag before closing channel to prevent new wg.Add calls
		atomic.StoreInt32(&s.shuttingDown, 1)
		close(s.shutdown)

		// Acquire lock to:
		// 1. Synchronize with Start() - ensures any in-flight wg.Add() completes
		// 2. Safely access and close the listener
		// Start() holds this lock while calling wg.Add(), so acquiring it here
		// guarantees all pending wg.Add() calls have completed before wg.Wait()
		s.mu.Lock()
		s.markStopped()
		listener := s.listener
		if listener != nil {
			if cerr := listener.Close(); cerr != nil && !errors.Is(cerr, net.ErrClosed) {
				err = fmt.Errorf("failed to close listener: %w", cerr)
			}
		}
		s.mu.Unlock()

		// Close all active connections
		s.connections.Range(func(_, value interface{}) bool {
			if conn, ok := value.(net.Conn); ok {
				if cerr := conn.Close(); cerr != nil {
					logrus.Debug("TCP connection cleanup failed during shutdown")
				}
			}
			return true
		})

		// Wait for all handlers to complete or timeout
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			logrus.Info("All TCP connections closed gracefully")
		case <-ctx.Done():
			err = fmt.Errorf("shutdown timeout exceeded, %d connections still active", atomic.LoadInt32(&s.activeConns))
		}
	})

	return err
}
