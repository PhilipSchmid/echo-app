package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/sirupsen/logrus"
)

// Manager manages all servers and handles graceful shutdown
type Manager struct {
	cfg      *config.Config
	servers  []Server
	wg       sync.WaitGroup
	shutdown chan struct{}
}

// Server interface for all server types
type Server interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
	Name() string
}

// NewManager creates a new server manager
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:      cfg,
		servers:  make([]Server, 0),
		shutdown: make(chan struct{}),
	}
}

// RegisterServer adds a server to be managed
func (m *Manager) RegisterServer(s Server) {
	m.servers = append(m.servers, s)
}

// Start starts all registered servers
func (m *Manager) Start(ctx context.Context) error {
	for _, srv := range m.servers {
		srv := srv // capture loop variable
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			logrus.Infof("Starting %s server...", srv.Name())
			if err := srv.Start(ctx); err != nil && err != http.ErrServerClosed {
				logrus.Errorf("%s server error: %v", srv.Name(), err)
			}
		}()
	}
	return nil
}

// Shutdown gracefully shuts down all servers
func (m *Manager) Shutdown(timeout time.Duration) error {
	close(m.shutdown)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	logrus.Info("Shutting down all servers...")

	var shutdownWg sync.WaitGroup
	errors := make(chan error, len(m.servers))

	for _, srv := range m.servers {
		srv := srv // capture loop variable
		shutdownWg.Add(1)
		go func() {
			defer shutdownWg.Done()
			if err := srv.Shutdown(ctx); err != nil {
				errors <- fmt.Errorf("%s shutdown error: %w", srv.Name(), err)
			}
		}()
	}

	// Wait for all shutdowns to complete
	shutdownWg.Wait()
	close(errors)

	// Collect any errors
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}

	// Wait for all server goroutines to finish
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logrus.Info("All servers shut down successfully")
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout exceeded")
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	return nil
}

// Wait blocks until all servers have stopped
func (m *Manager) Wait() {
	m.wg.Wait()
}
