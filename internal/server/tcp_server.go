package server

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/handlers"
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
	mu           sync.RWMutex // Protects listener and ctx
}

// NewTCPServer creates a new TCP server
func NewTCPServer(cfg *config.Config) *TCPServer {
	return &TCPServer{
		cfg:        cfg,
		listenAddr: ":" + cfg.TCPPort,
		shutdown:   make(chan struct{}),
	}
}

// Name returns the server name
func (s *TCPServer) Name() string {
	return "TCP"
}

// Start starts the TCP server
func (s *TCPServer) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.listenAddr, err)
	}

	// Store context and listener with mutex protection
	s.mu.Lock()
	s.ctx = ctx
	s.listener = listener
	s.mu.Unlock()

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
				logrus.Errorf("Failed to set accept deadline: %v", err)
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
					logrus.Errorf("Failed to accept connection: %v", err)
					continue
				}
			}

			// Use mutex to make shutdown check and wg.Add atomic
			// This prevents race between wg.Add() here and wg.Wait() in Shutdown
			s.mu.Lock()
			if atomic.LoadInt32(&s.shuttingDown) == 1 {
				s.mu.Unlock()
				if err := conn.Close(); err != nil {
					logrus.Errorf("Failed to close connection during shutdown: %v", err)
				}
				return nil
			}

			// Check connection limit
			currentConns := atomic.LoadInt32(&s.activeConns)
			if currentConns >= maxTCPConnections {
				s.mu.Unlock()
				logrus.Warnf("Connection limit reached (%d), rejecting new connection", maxTCPConnections)
				if err := conn.Close(); err != nil {
					logrus.Errorf("Failed to close rejected connection: %v", err)
				}
				continue
			}

			// Handle connection - wg.Add protected by mutex
			s.wg.Add(1)
			s.mu.Unlock()
			atomic.AddInt32(&s.activeConns, 1)
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
	s.connections.Store(connID, conn)
	defer s.connections.Delete(connID)

	// Set connection timeout
	if err := conn.SetDeadline(time.Now().Add(tcpTimeout)); err != nil {
		logrus.Errorf("Failed to set connection deadline: %v", err)
	}

	// Get context safely
	s.mu.RLock()
	ctx := s.ctx
	s.mu.RUnlock()

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

		// Close listener
		s.mu.RLock()
		listener := s.listener
		s.mu.RUnlock()

		if listener != nil {
			if cerr := listener.Close(); cerr != nil {
				err = fmt.Errorf("failed to close listener: %w", cerr)
			}
		}

		// Close all active connections
		s.connections.Range(func(key, value interface{}) bool {
			if conn, ok := value.(net.Conn); ok {
				if cerr := conn.Close(); cerr != nil {
					logrus.Errorf("Failed to close connection %v: %v", key, cerr)
				}
			}
			return true
		})

		// Acquire and release lock to ensure no new wg.Add() calls can happen.
		// This acts as a memory barrier synchronizing with Start() which holds
		// the lock during wg.Add(). The empty critical section is intentional.
		s.mu.Lock()   //nolint:staticcheck // SA2001: intentional sync barrier
		s.mu.Unlock() //nolint:staticcheck // SA2001: intentional sync barrier

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
