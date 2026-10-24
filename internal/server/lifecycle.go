package server

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// lifecycle serializes binding with shutdown and reports a bound listener.
type lifecycle struct {
	mu      sync.Mutex
	ready   chan struct{}
	stopped bool
}

func newLifecycle() lifecycle { return lifecycle{ready: make(chan struct{})} }

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
