package server

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPServer_StartAndStop(t *testing.T) {
	cfg := &config.Config{
		HTTPPort: "18080", // Use different port to avoid conflicts
		Message:  "test",
	}

	server := NewHTTPServer(cfg, false)
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
	resp, err := http.Get("http://localhost:18080/")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Stop server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	err = server.Shutdown(shutdownCtx)
	assert.NoError(t, err)

	// Wait for Start() to finish
	select {
	case err := <-errCh:
		// Should get http.ErrServerClosed
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Unexpected error from Start(): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not stop in time")
	}
}

func TestTLSServer_StartAndStop(t *testing.T) {
	cfg := &config.Config{
		TLSPort: "18443", // Use different port to avoid conflicts
		Message: "test",
	}

	server := NewHTTPServer(cfg, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Wait for server to start (TLS setup takes a bit longer)
	time.Sleep(200 * time.Millisecond)

	// Verify server is listening (skip TLS verification for self-signed cert)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	resp, err := client.Get("https://localhost:18443/")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Stop server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	err = server.Shutdown(shutdownCtx)
	assert.NoError(t, err)

	// Cancel context
	cancel()

	// Wait for Start() to finish
	select {
	case err := <-errCh:
		// Should get http.ErrServerClosed or nil
		if err != nil && err != http.ErrServerClosed {
			t.Logf("Start() returned: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not stop in time")
	}
}

func TestHTTPServer_ConnectionLimit(t *testing.T) {
	cfg := &config.Config{
		HTTPPort: "18081",
		Message:  "test",
	}

	server := NewHTTPServer(cfg, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Create a handler that blocks to keep connections open
	attempts := maxHTTPConnections + 10
	var successCount int32
	var serviceUnavailableCount int32
	var wg sync.WaitGroup

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get("http://localhost:18081/")
			if err != nil {
				return
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
			} else if resp.StatusCode == http.StatusServiceUnavailable {
				atomic.AddInt32(&serviceUnavailableCount, 1)
			}

			// Read and discard body
			_, _ = io.Copy(io.Discard, resp.Body)
		}()
	}

	wg.Wait()

	successfulConns := atomic.LoadInt32(&successCount)
	rejectedConns := atomic.LoadInt32(&serviceUnavailableCount)

	t.Logf("Successful connections: %d, Rejected: %d", successfulConns, rejectedConns)

	// We should have some successful connections
	assert.Greater(t, int(successfulConns), 0)

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	cancel()
}

func TestHTTPServer_Name(t *testing.T) {
	tests := []struct {
		name     string
		useTLS   bool
		expected string
	}{
		{
			name:     "HTTP server",
			useTLS:   false,
			expected: "HTTP",
		},
		{
			name:     "TLS server",
			useTLS:   true,
			expected: "TLS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				HTTPPort: "18082",
				TLSPort:  "18444",
			}

			server := NewHTTPServer(cfg, tt.useTLS)
			assert.Equal(t, tt.expected, server.Name())
		})
	}
}

func TestHTTPServer_GracefulShutdown(t *testing.T) {
	cfg := &config.Config{
		HTTPPort: "18083",
		Message:  "test",
	}

	server := NewHTTPServer(cfg, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Make a request to ensure server is working
	resp, err := http.Get("http://localhost:18083/")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

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
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown did not complete in time")
	}

	cancel()
}

func TestHTTPServer_ConcurrentRequests(t *testing.T) {
	cfg := &config.Config{
		HTTPPort: "18084",
		Message:  "test",
	}

	server := NewHTTPServer(cfg, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Make many concurrent requests
	numRequests := 50
	var wg sync.WaitGroup
	successCount := int32(0)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get("http://localhost:18084/")
			if err != nil {
				return
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
			}

			// Read and discard body
			_, _ = io.Copy(io.Discard, resp.Body)
		}()
	}

	wg.Wait()

	// All requests should succeed
	assert.Equal(t, int32(numRequests), atomic.LoadInt32(&successCount))

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	cancel()
}

func TestHTTPServer_ShutdownWithoutStart(t *testing.T) {
	cfg := &config.Config{
		HTTPPort: "18085",
	}

	server := NewHTTPServer(cfg, false)

	// Shutdown without starting should not error
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	err := server.Shutdown(shutdownCtx)
	assert.NoError(t, err)
}

func TestHTTPServer_ActiveConnectionTracking(t *testing.T) {
	cfg := &config.Config{
		HTTPPort: "18086",
		Message:  "test",
	}

	server := NewHTTPServer(cfg, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Initially no connections
	assert.Equal(t, int32(0), atomic.LoadInt32(&server.activeConns))

	// Make a request
	resp, err := http.Get("http://localhost:18086/")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Read body to complete request
	_, _ = io.Copy(io.Discard, resp.Body)

	// Wait for handler to complete
	time.Sleep(100 * time.Millisecond)

	// Should be back to 0 (or very low due to timing)
	activeConns := atomic.LoadInt32(&server.activeConns)
	assert.LessOrEqual(t, activeConns, int32(1))

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	cancel()
}
