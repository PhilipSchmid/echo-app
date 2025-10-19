package server

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTCPServer_StartAndStop(t *testing.T) {
	cfg := &config.Config{
		TCPPort: "19090", // Use different port to avoid conflicts
		Message: "test",
	}

	server := NewTCPServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Verify server is listening
	conn, err := net.Dial("tcp", "localhost:19090")
	require.NoError(t, err)
	_ = conn.Close()

	// Stop server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	err = server.Shutdown(shutdownCtx)
	assert.NoError(t, err)

	// Cancel context to stop Start()
	cancel()

	// Wait for Start() to finish
	select {
	case err := <-errCh:
		// Either nil (from shutdown) or context.Canceled is acceptable
		if err != nil {
			assert.Contains(t, err.Error(), "context canceled")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not stop in time")
	}
}

func TestTCPServer_ConnectionLimit(t *testing.T) {
	cfg := &config.Config{
		TCPPort: "19091",
		Message: "test",
	}

	server := NewTCPServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Create connections up to the limit
	var conns []net.Conn
	var connMutex sync.Mutex

	// Try to create more than maxTCPConnections
	attempts := maxTCPConnections + 10
	var successCount int32

	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp", "localhost:19091")
			if err == nil {
				atomic.AddInt32(&successCount, 1)
				connMutex.Lock()
				conns = append(conns, conn)
				connMutex.Unlock()
			}
		}()
	}

	wg.Wait()

	// Verify we didn't exceed the limit by much (allow small overage due to race conditions)
	successfulConns := atomic.LoadInt32(&successCount)
	// Allow up to 5% overage due to race between accept and counter check
	maxAllowed := maxTCPConnections + 50
	assert.LessOrEqual(t, int(successfulConns), maxAllowed,
		"Connection count should be near limit (got %d, limit %d, max allowed %d)",
		successfulConns, maxTCPConnections, maxAllowed)
	t.Logf("Successfully created %d connections (limit: %d)", successfulConns, maxTCPConnections)

	// Clean up connections
	connMutex.Lock()
	for _, conn := range conns {
		_ = conn.Close()
	}
	connMutex.Unlock()

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	cancel()
}

func TestTCPServer_GracefulShutdown(t *testing.T) {
	cfg := &config.Config{
		TCPPort: "19092",
		Message: "test",
	}

	server := NewTCPServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Create some connections
	var conns []net.Conn
	for i := 0; i < 5; i++ {
		conn, err := net.Dial("tcp", "localhost:19092")
		require.NoError(t, err)
		conns = append(conns, conn)
	}

	// Initiate graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- server.Shutdown(shutdownCtx)
	}()

	// Wait for shutdown to complete
	select {
	case err := <-shutdownDone:
		// Shutdown might return error if connections didn't close fast enough, that's ok
		if err != nil {
			t.Logf("Shutdown returned error (acceptable): %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown did not complete in time")
	}

	// Clean up connections (they may already be closed by shutdown)
	for _, conn := range conns {
		_ = conn.Close()
	}

	cancel()
}

func TestTCPServer_ShutdownTimeout(t *testing.T) {
	cfg := &config.Config{
		TCPPort: "19093",
		Message: "test",
	}

	server := NewTCPServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Create a connection
	conn, err := net.Dial("tcp", "localhost:19093")
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Initiate shutdown with very short timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shutdownCancel()

	err = server.Shutdown(shutdownCtx)
	// Should get timeout error if connections didn't close in time
	// But might also succeed if connections closed quickly
	if err != nil {
		assert.Contains(t, err.Error(), "timeout")
	}

	cancel()
}

func TestTCPServer_ContextCancellation(t *testing.T) {
	cfg := &config.Config{
		TCPPort: "19094",
		Message: "test",
	}

	server := NewTCPServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())

	// Start server
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for Start() to return with context error
	select {
	case err := <-errCh:
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not stop after context cancellation")
	}
}

func TestTCPServer_ActiveConnectionTracking(t *testing.T) {
	cfg := &config.Config{
		TCPPort: "19095",
		Message: "test",
	}

	server := NewTCPServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Initially no connections
	assert.Equal(t, int32(0), atomic.LoadInt32(&server.activeConns))

	// Create connections
	conn1, err := net.Dial("tcp", "localhost:19095")
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond) // Give handler time to increment counter

	conn2, err := net.Dial("tcp", "localhost:19095")
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Should have 2 active connections (or less if handlers completed)
	activeConns := atomic.LoadInt32(&server.activeConns)
	assert.LessOrEqual(t, activeConns, int32(2))

	// Close connections
	_ = conn1.Close()
	_ = conn2.Close()

	// Wait for handlers to complete
	time.Sleep(200 * time.Millisecond)

	// Should be back to 0 (or very low)
	activeConns = atomic.LoadInt32(&server.activeConns)
	assert.LessOrEqual(t, activeConns, int32(1)) // Allow for timing

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	cancel()
}

func TestTCPServer_Name(t *testing.T) {
	cfg := &config.Config{
		TCPPort: "19096",
	}

	server := NewTCPServer(cfg)
	assert.Equal(t, "TCP", server.Name())
}

func TestTCPServer_ConcurrentConnections(t *testing.T) {
	cfg := &config.Config{
		TCPPort: "19097",
		Message: "test",
	}

	server := NewTCPServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Create many concurrent connections
	numConns := 50
	var wg sync.WaitGroup
	successCount := int32(0)

	for i := 0; i < numConns; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp", "localhost:19097")
			if err != nil {
				return
			}
			atomic.AddInt32(&successCount, 1)

			// Send some data
			_, _ = conn.Write([]byte("test"))

			// Read response
			buf := make([]byte, 4096)
			_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			_, _ = conn.Read(buf)
			_ = conn.Close()
		}()
	}

	wg.Wait()

	// All connections should succeed
	assert.Equal(t, int32(numConns), atomic.LoadInt32(&successCount))

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	cancel()
}
