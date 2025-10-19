package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetricsServer(t *testing.T) {
	cfg := &config.Config{
		MetricsPort: "13000",
	}

	server := NewMetricsServer(cfg)

	assert.NotNil(t, server)
	assert.Equal(t, cfg, server.cfg)
	assert.Equal(t, ":13000", server.listenAddr)
}

func TestMetricsServer_Name(t *testing.T) {
	cfg := &config.Config{
		MetricsPort: "13001",
	}

	server := NewMetricsServer(cfg)
	assert.Equal(t, "Metrics", server.Name())
}

func TestMetricsServer_StartAndShutdown(t *testing.T) {
	cfg := &config.Config{
		MetricsPort: "13002",
	}

	server := NewMetricsServer(cfg)
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
	resp, err := http.Get("http://localhost:13002/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Stop server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	err = server.Shutdown(shutdownCtx)
	assert.NoError(t, err)

	// Wait for Start() to finish
	select {
	case err := <-errCh:
		// Should get http.ErrServerClosed or nil
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Unexpected error from Start(): %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Server did not stop in time")
	}
}

func TestMetricsServer_HealthEndpoint(t *testing.T) {
	cfg := &config.Config{
		MetricsPort: "13003",
	}

	server := NewMetricsServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Test health endpoint
	resp, err := http.Get("http://localhost:13003/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "OK", string(body))

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

func TestMetricsServer_ReadyEndpoint(t *testing.T) {
	cfg := &config.Config{
		MetricsPort: "13004",
	}

	server := NewMetricsServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Test ready endpoint
	resp, err := http.Get("http://localhost:13004/ready")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "Ready", string(body))

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

func TestMetricsServer_MetricsEndpoint(t *testing.T) {
	cfg := &config.Config{
		MetricsPort: "13005",
	}

	server := NewMetricsServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Test metrics endpoint
	resp, err := http.Get("http://localhost:13005/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Verify it returns Prometheus metrics format
	bodyStr := string(body)
	// Should contain go metrics at minimum
	assert.Contains(t, bodyStr, "go_")

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

func TestMetricsServer_MetricsTimeout(t *testing.T) {
	cfg := &config.Config{
		MetricsPort: "13006",
	}

	server := NewMetricsServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Test metrics endpoint with very slow client
	// The timeout is set to 10 seconds in the server, so this should succeed
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get("http://localhost:13006/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Read response
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

func TestMetricsServer_ShutdownWithoutStart(t *testing.T) {
	cfg := &config.Config{
		MetricsPort: "13007",
	}

	server := NewMetricsServer(cfg)

	// Shutdown without starting should not error
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	err := server.Shutdown(shutdownCtx)
	assert.NoError(t, err)
}

func TestMetricsServer_GracefulShutdown(t *testing.T) {
	cfg := &config.Config{
		MetricsPort: "13008",
	}

	server := NewMetricsServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Make a request to ensure server is working
	resp, err := http.Get("http://localhost:13008/health")
	require.NoError(t, err)
	defer resp.Body.Close()
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
}

func TestMetricsServer_MultipleEndpoints(t *testing.T) {
	cfg := &config.Config{
		MetricsPort: "13009",
	}

	server := NewMetricsServer(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Test all endpoints
	tests := []struct {
		name           string
		endpoint       string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "health endpoint",
			endpoint:       "/health",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
		{
			name:           "ready endpoint",
			endpoint:       "/ready",
			expectedStatus: http.StatusOK,
			expectedBody:   "Ready",
		},
		{
			name:           "metrics endpoint",
			endpoint:       "/metrics",
			expectedStatus: http.StatusOK,
			expectedBody:   "go_", // Should contain go metrics
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get("http://localhost:13009" + tt.endpoint)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tt.expectedBody != "" {
				if strings.Contains(tt.expectedBody, "_") {
					// Partial match for metrics
					assert.Contains(t, string(body), tt.expectedBody)
				} else {
					// Exact match for health/ready
					assert.Equal(t, tt.expectedBody, string(body))
				}
			}
		})
	}

	// Shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}
