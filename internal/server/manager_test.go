package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/health"
	"github.com/stretchr/testify/require"
)

type mockServer struct {
	ready         chan struct{}
	done          chan struct{}
	once          sync.Once
	startError    error
	shutdownError error
	shutdownDelay time.Duration
	startFn       func(context.Context) error
}

func newMockServer(_ string) *mockServer {
	return &mockServer{ready: make(chan struct{}), done: make(chan struct{})}
}
func (m *mockServer) Ready() <-chan struct{} { return m.ready }
func (m *mockServer) Name() string           { return "mock" }
func (m *mockServer) Start(ctx context.Context) error {
	if m.startFn != nil {
		return m.startFn(ctx)
	}
	if m.startError != nil {
		return m.startError
	}
	close(m.ready)
	select {
	case <-m.done:
		return http.ErrServerClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (m *mockServer) Shutdown(context.Context) error {
	time.Sleep(m.shutdownDelay)
	m.once.Do(func() { close(m.done) })
	return m.shutdownError
}

func TestManager_StartAndShutdown(t *testing.T) {
	checker := health.NewChecker(config.ExternalReadinessProbe{})
	manager := NewManager(&config.Config{}, checker)
	recorder := httptest.NewRecorder()
	checker.ReadyHandler(recorder, httptest.NewRequest("GET", "/ready", nil))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	manager.RegisterServer(newMockServer("first"))
	manager.RegisterServer(newMockServer("second"))
	require.NoError(t, manager.Start(context.Background()))
	recorder = httptest.NewRecorder()
	checker.ReadyHandler(recorder, httptest.NewRequest("GET", "/ready", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, manager.Shutdown(time.Second))
	require.NoError(t, manager.Shutdown(time.Second))
	recorder = httptest.NewRecorder()
	checker.ReadyHandler(recorder, httptest.NewRequest("GET", "/ready", nil))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	manager.Wait()
}
func TestManager_StartFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	checker := health.NewChecker(config.ExternalReadinessProbe{})
	manager := NewManager(&config.Config{}, checker)
	first := NewHTTPServer(&config.Config{HTTPPort: "0"}, false)
	bad := NewHTTPServer(&config.Config{}, false)
	bad.listenAddr = ln.Addr().String()
	manager.RegisterServer(first)
	manager.RegisterServer(bad)
	require.Error(t, manager.Start(context.Background()))
	recorder := httptest.NewRecorder()
	checker.ReadyHandler(recorder, httptest.NewRequest("GET", "/ready", nil))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	conn, err := net.DialTimeout("tcp", first.netListener.Addr().String(), time.Second)
	if conn != nil {
		_ = conn.Close()
	}
	require.Error(t, err, "a partial startup must close previously bound listeners")
}
func TestManager_RuntimeFailure(t *testing.T) {
	checker := health.NewChecker(config.ExternalReadinessProbe{})
	manager := NewManager(&config.Config{}, checker)
	srv := NewHTTPServer(&config.Config{HTTPPort: "0"}, false)
	manager.RegisterServer(srv)
	require.NoError(t, manager.Start(context.Background()))
	require.NoError(t, srv.netListener.Close())
	select {
	case err := <-manager.Errors():
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("listener failure was lost")
	}
	recorder := httptest.NewRecorder()
	checker.ReadyHandler(recorder, httptest.NewRequest("GET", "/ready", nil))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.NoError(t, manager.Shutdown(time.Second))
}
func TestManager_ShutdownTimeout(t *testing.T) {
	manager := NewManager(&config.Config{}, nil)
	srv := newMockServer("slow")
	srv.shutdownDelay = 150 * time.Millisecond
	manager.RegisterServer(srv)
	require.NoError(t, manager.Start(context.Background()))
	start := time.Now()
	require.ErrorContains(t, manager.Shutdown(10*time.Millisecond), "timeout")
	require.Less(t, time.Since(start), 100*time.Millisecond)
	manager.Wait()
}
func TestManager_ShutdownErrors(t *testing.T) {
	manager := NewManager(&config.Config{}, nil)
	srv := newMockServer("broken")
	srv.shutdownError = errors.New("expected shutdown failure")
	manager.RegisterServer(srv)
	require.NoError(t, manager.Start(context.Background()))
	require.ErrorContains(t, manager.Shutdown(time.Second), "expected shutdown failure")
}
func TestManager_ShutdownBeforeStart(t *testing.T) {
	manager := NewManager(&config.Config{}, nil)
	manager.RegisterServer(newMockServer("server"))
	require.NoError(t, manager.Shutdown(time.Second))
	require.Error(t, manager.Start(context.Background()))
}

func TestManager_DuplicateStartKeepsServing(t *testing.T) {
	manager := NewManager(&config.Config{}, nil)
	srv := NewHTTPServer(&config.Config{HTTPPort: "0"}, false)
	srv.listenAddr = "127.0.0.1:0"
	manager.RegisterServer(srv)
	require.NoError(t, manager.Start(context.Background()))
	defer func() { _ = manager.Shutdown(time.Second) }()
	require.ErrorContains(t, manager.Start(context.Background()), "already started")
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + srv.netListener.Addr().String())
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestManager_ConsumedFailureDuringStartup(t *testing.T) {
	manager := NewManager(&config.Config{}, nil)
	first := NewHTTPServer(&config.Config{HTTPPort: "0"}, false)
	second := newMockServer("second")
	entered, ready := make(chan struct{}), make(chan struct{})
	second.startFn = func(context.Context) error {
		close(entered)
		<-ready
		close(second.ready)
		<-second.done
		return http.ErrServerClosed
	}
	manager.RegisterServer(first)
	manager.RegisterServer(second)
	done := make(chan error, 1)
	go func() { done <- manager.Start(context.Background()) }()
	<-entered
	require.NoError(t, first.netListener.Close())
	select {
	case err := <-manager.Errors():
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("listener failure not reported")
	}
	close(ready)
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("startup blocked after Errors subscriber consumed failure")
	}
}
