package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
)

// lifecycle serializes binding with shutdown and reports a bound listener.
type lifecycle struct {
	mu      sync.Mutex
	ready   chan struct{}
	stopped bool
	name    string
}

func newLifecycle(name string) lifecycle {
	metrics.ListenerUp.WithLabelValues(name).Set(0)
	return lifecycle{ready: make(chan struct{}), name: name}
}

// markReady and markStopped run under mu so shutdown cannot leave a stale up gauge.
func (s *lifecycle) markReady() {
	metrics.ListenerUp.WithLabelValues(s.name).Set(1)
	close(s.ready)
}

func (s *lifecycle) markStopped() {
	s.stopped = true
	metrics.ListenerUp.WithLabelValues(s.name).Set(0)
}

func (s *lifecycle) markFinished() {
	s.mu.Lock()
	metrics.ListenerUp.WithLabelValues(s.name).Set(0)
	s.mu.Unlock()
}

func (s *lifecycle) Ready() <-chan struct{} { return s.ready }

// checkStart is called while holding mu, before opening a socket.
func (s *lifecycle) checkStart(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.stopped {
		return http.ErrServerClosed
	}
	select {
	case <-s.ready:
		return http.ErrServerClosed
	default:
		return nil
	}
}

func serveWithContext(ctx context.Context, serve func() error, shutdown func(context.Context) error) error {
	stop := context.AfterFunc(ctx, func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = shutdown(shutdownCtx)
	})
	defer stop()
	return serve()
}
