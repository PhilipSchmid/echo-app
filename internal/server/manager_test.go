package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockServer is a mock implementation of the Server interface for testing
type mockServer struct {
	name          string
	startCalled   int32
	shutdownCalled int32
	startDelay    time.Duration
	shutdownDelay time.Duration
	startError    error
	shutdownError error
	blockStart    bool
	shutdownCh    chan struct{}
	shutdownOnce  sync.Once
	ctx           context.Context
	cancel        context.CancelFunc
}

func newMockServer(name string) *mockServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockServer{
		name:       name,
		shutdownCh: make(chan struct{}),
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (m *mockServer) Start(ctx context.Context) error {
	atomic.AddInt32(&m.startCalled, 1)

	if m.startDelay > 0 {
		time.Sleep(m.startDelay)
	}

	if m.blockStart {
		// Block until shutdown is called or context is cancelled
		select {
		case <-m.shutdownCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-m.ctx.Done():
			return nil
		}
	}

	return m.startError
}

func (m *mockServer) Shutdown(ctx context.Context) error {
	atomic.AddInt32(&m.shutdownCalled, 1)

	if m.shutdownDelay > 0 {
		time.Sleep(m.shutdownDelay)
	}

	// Signal Start() to complete (only once)
	m.shutdownOnce.Do(func() {
		m.cancel()
		close(m.shutdownCh)
	})

	return m.shutdownError
}

func (m *mockServer) Name() string {
	return m.name
}

func TestNewManager(t *testing.T) {
	cfg := &config.Config{}
	manager := NewManager(cfg)

	assert.NotNil(t, manager)
	assert.Equal(t, cfg, manager.cfg)
	assert.NotNil(t, manager.servers)
	assert.Equal(t, 0, len(manager.servers))
	assert.NotNil(t, manager.shutdown)
}

func TestManager_RegisterServer(t *testing.T) {
	cfg := &config.Config{}
	manager := NewManager(cfg)

	// Register first server
	srv1 := newMockServer("server1")
	manager.RegisterServer(srv1)
	assert.Equal(t, 1, len(manager.servers))

	// Register second server
	srv2 := newMockServer("server2")
	manager.RegisterServer(srv2)
	assert.Equal(t, 2, len(manager.servers))

	// Verify servers are in order
	assert.Equal(t, "server1", manager.servers[0].Name())
	assert.Equal(t, "server2", manager.servers[1].Name())
}

func TestManager_StartAndShutdown(t *testing.T) {
	cfg := &config.Config{}
	manager := NewManager(cfg)

	// Create mock servers
	srv1 := newMockServer("server1")
	srv1.blockStart = true
	srv2 := newMockServer("server2")
	srv2.blockStart = true

	manager.RegisterServer(srv1)
	manager.RegisterServer(srv2)

	// Start servers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)

	// Wait for servers to start
	time.Sleep(100 * time.Millisecond)

	// Verify servers were started
	assert.Equal(t, int32(1), atomic.LoadInt32(&srv1.startCalled))
	assert.Equal(t, int32(1), atomic.LoadInt32(&srv2.startCalled))

	// Shutdown servers
	err = manager.Shutdown(5 * time.Second)
	assert.NoError(t, err)

	// Verify servers were shut down
	assert.Equal(t, int32(1), atomic.LoadInt32(&srv1.shutdownCalled))
	assert.Equal(t, int32(1), atomic.LoadInt32(&srv2.shutdownCalled))

	cancel()
}

func TestManager_ShutdownTimeout(t *testing.T) {
	cfg := &config.Config{}
	manager := NewManager(cfg)

	// Create a mock server that takes too long to shut down
	srv := newMockServer("slow-server")
	srv.blockStart = true
	srv.shutdownDelay = 3 * time.Second

	manager.RegisterServer(srv)

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Try to shutdown with very short timeout
	err = manager.Shutdown(100 * time.Millisecond)

	// Should get timeout error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")

	cancel()

	// Give server time to finish
	time.Sleep(3 * time.Second)
}

func TestManager_ShutdownErrors(t *testing.T) {
	cfg := &config.Config{}
	manager := NewManager(cfg)

	// Create mock servers with shutdown errors
	srv1 := newMockServer("server1")
	srv1.blockStart = true
	srv1.shutdownError = errors.New("shutdown error 1")

	srv2 := newMockServer("server2")
	srv2.blockStart = true
	srv2.shutdownError = errors.New("shutdown error 2")

	manager.RegisterServer(srv1)
	manager.RegisterServer(srv2)

	// Start servers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)

	// Wait for servers to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown servers
	err = manager.Shutdown(5 * time.Second)

	// Should get shutdown errors
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shutdown errors")

	cancel()
}

func TestManager_Wait(t *testing.T) {
	cfg := &config.Config{}
	manager := NewManager(cfg)

	// Create mock server that completes quickly
	srv := newMockServer("server")
	srv.startDelay = 200 * time.Millisecond

	manager.RegisterServer(srv)

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)

	// Wait for all servers to complete
	done := make(chan struct{})
	go func() {
		manager.Wait()
		close(done)
	}()

	// Should complete after start delay
	select {
	case <-done:
		// Success - Wait() returned
	case <-time.After(1 * time.Second):
		t.Fatal("Wait() did not return in time")
	}
}

func TestManager_MultipleServers(t *testing.T) {
	cfg := &config.Config{}
	manager := NewManager(cfg)

	// Create multiple mock servers
	numServers := 5
	for i := 0; i < numServers; i++ {
		srv := newMockServer("server")
		srv.blockStart = true
		manager.RegisterServer(srv)
	}

	// Start all servers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)

	// Wait for servers to start
	time.Sleep(100 * time.Millisecond)

	// Verify all servers started
	for _, srv := range manager.servers {
		mockSrv := srv.(*mockServer)
		assert.Equal(t, int32(1), atomic.LoadInt32(&mockSrv.startCalled))
	}

	// Shutdown all servers
	err = manager.Shutdown(5 * time.Second)
	assert.NoError(t, err)

	// Verify all servers shut down
	for _, srv := range manager.servers {
		mockSrv := srv.(*mockServer)
		assert.Equal(t, int32(1), atomic.LoadInt32(&mockSrv.shutdownCalled))
	}

	cancel()
}

func TestManager_EmptyManager(t *testing.T) {
	cfg := &config.Config{}
	manager := NewManager(cfg)

	// Start with no servers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)

	// Shutdown with no servers
	err = manager.Shutdown(5 * time.Second)
	assert.NoError(t, err)

	// Wait with no servers
	manager.Wait() // Should return immediately
}

func TestManager_ShutdownBeforeStart(t *testing.T) {
	cfg := &config.Config{}
	manager := NewManager(cfg)

	srv := newMockServer("server")
	manager.RegisterServer(srv)

	// Shutdown before starting
	err := manager.Shutdown(5 * time.Second)
	assert.NoError(t, err)

	// Verify server was not started
	assert.Equal(t, int32(0), atomic.LoadInt32(&srv.startCalled))
	// But shutdown was still called
	assert.Equal(t, int32(1), atomic.LoadInt32(&srv.shutdownCalled))
}

func TestManager_ConcurrentShutdown(t *testing.T) {
	cfg := &config.Config{}
	manager := NewManager(cfg)

	// Create servers with different shutdown delays
	srv1 := newMockServer("fast-server")
	srv1.blockStart = true
	srv1.shutdownDelay = 50 * time.Millisecond

	srv2 := newMockServer("slow-server")
	srv2.blockStart = true
	srv2.shutdownDelay = 200 * time.Millisecond

	manager.RegisterServer(srv1)
	manager.RegisterServer(srv2)

	// Start servers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := manager.Start(ctx)
	require.NoError(t, err)

	// Wait for servers to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown - should wait for both to complete
	start := time.Now()
	err = manager.Shutdown(5 * time.Second)
	duration := time.Since(start)

	assert.NoError(t, err)

	// Should take at least as long as the slowest server
	assert.GreaterOrEqual(t, duration, 200*time.Millisecond)

	// Verify both servers shut down
	assert.Equal(t, int32(1), atomic.LoadInt32(&srv1.shutdownCalled))
	assert.Equal(t, int32(1), atomic.LoadInt32(&srv2.shutdownCalled))

	cancel()
}
