package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/health"
)

type Manager struct {
	cfg          *config.Config
	health       *health.Checker
	servers      []Server
	wg           sync.WaitGroup
	mu           sync.Mutex
	shutdown     chan struct{}
	shutdownOnce sync.Once
	errors       chan error
	failed       error
	started      bool
}

type Server interface {
	Start(context.Context) error
	Shutdown(context.Context) error
	Ready() <-chan struct{}
	Name() string
}

func NewManager(cfg *config.Config, checker *health.Checker) *Manager {
	if checker != nil {
		checker.SetReady(false, "starting listeners")
	}
	return &Manager{cfg: cfg, health: checker, servers: make([]Server, 0), shutdown: make(chan struct{}), errors: make(chan error, 1)}
}

func (m *Manager) RegisterServer(s Server) { m.servers = append(m.servers, s) }

// Errors reports the first unexpected listener failure after startup.
func (m *Manager) Errors() <-chan error { return m.errors }

func (m *Manager) Start(ctx context.Context) (err error) {
	m.mu.Lock()
	select {
	case <-m.shutdown:
		m.mu.Unlock()
		return http.ErrServerClosed
	default:
	}
	if m.started {
		m.mu.Unlock()
		return fmt.Errorf("servers already started")
	}
	m.started = true
	m.mu.Unlock()
	defer func() {
		if err != nil {
			_ = m.Shutdown(30 * time.Second)
		}
	}()

	for _, srv := range m.servers {
		result := make(chan error, 1)
		m.mu.Lock()
		select {
		case <-m.shutdown:
			m.mu.Unlock()
			return http.ErrServerClosed
		default:
		}
		m.wg.Add(1)
		m.mu.Unlock()
		go func(srv Server) {
			defer m.wg.Done()
			err := srv.Start(ctx)
			if err == nil {
				err = fmt.Errorf("listener stopped unexpectedly")
			}
			result <- err
			if errors.Is(err, http.ErrServerClosed) || errors.Is(err, context.Canceled) {
				return
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			select {
			case <-m.shutdown:
				return
			default:
			}
			if m.failed == nil {
				m.failed = fmt.Errorf("%s server: %w", srv.Name(), err)
			}
			if m.health != nil {
				m.health.SetReady(false, srv.Name()+" listener failed")
			}
			select {
			case m.errors <- m.failed:
			default:
			}
		}(srv)
		select {
		case <-srv.Ready():
		case err := <-result:
			return fmt.Errorf("start %s server: %w", srv.Name(), err)
		case <-ctx.Done():
			return ctx.Err()
		case <-m.shutdown:
			return http.ErrServerClosed
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failed != nil {
		return m.failed
	}
	select {
	case <-m.shutdown:
		return http.ErrServerClosed
	default:
	}
	if m.health != nil {
		m.health.SetReady(true, "")
	}
	return nil
}

func (m *Manager) Shutdown(timeout time.Duration) error {
	m.mu.Lock()
	m.shutdownOnce.Do(func() { close(m.shutdown) })
	if m.health != nil {
		m.health.SetReady(false, "shutting down")
	}
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result := make(chan error, len(m.servers))
	for _, srv := range m.servers {
		go func(srv Server) { result <- srv.Shutdown(ctx) }(srv)
	}
	var errs []error
	for range m.servers {
		select {
		case err := <-result:
			if err != nil {
				errs = append(errs, err)
			}
		case <-ctx.Done():
			return fmt.Errorf("shutdown timeout exceeded: %w", ctx.Err())
		}
	}
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
		if len(errs) > 0 {
			return fmt.Errorf("shutdown errors: %w", errors.Join(errs...))
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout exceeded: %w", ctx.Err())
	}
}

func (m *Manager) Wait() { m.wg.Wait() }
