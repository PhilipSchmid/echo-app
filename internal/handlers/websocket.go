package handlers

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/metrics"
	"github.com/coder/websocket"
)

// NewWebSocketHandler returns bounded message echo and a shutdown hook for hijacked connections.
func NewWebSocketHandler(cfg *config.Config) (http.Handler, func()) {
	var mu sync.Mutex
	closed := false
	connections := make(map[*websocket.Conn]struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.ProtoMajor != 1 || r.ProtoMinor < 1 {
			http.Error(w, "WebSocket requires HTTP/1.1", http.StatusUpgradeRequired)
			return
		}
		if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			http.Error(w, "WebSocket handshake must not contain a body", http.StatusBadRequest)
			return
		}
		if value := r.Header.Get("Origin"); value != "" {
			origin, err := url.Parse(value)
			if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") {
				http.Error(w, "WebSocket origin is not authorized", http.StatusForbidden)
				return
			}
		}
		mu.Lock()
		stopped := closed
		mu.Unlock()
		if stopped {
			http.Error(w, "Server is shutting down", http.StatusServiceUnavailable)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		conn.SetReadLimit(min(cfg.MaxRequestSize, int64(maxResponseSize)))
		mu.Lock()
		if closed {
			mu.Unlock()
			return
		}
		connections[conn] = struct{}{}
		mu.Unlock()
		defer func() {
			mu.Lock()
			delete(connections, conn)
			mu.Unlock()
		}()
		metrics.ConnectionOpened("WebSocket")
		defer metrics.ConnectionClosed("WebSocket")
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			messageType, data, err := conn.Read(ctx)
			cancel()
			if err != nil {
				return
			}
			if messageType == websocket.MessageText && !utf8.Valid(data) {
				_ = conn.Close(websocket.StatusInvalidFramePayloadData, "invalid UTF-8")
				return
			}
			ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
			err = conn.Write(ctx, messageType, data)
			cancel()
			if err != nil {
				return
			}
		}
	})
	return handler, func() {
		mu.Lock()
		closed = true
		active := make([]*websocket.Conn, 0, len(connections))
		for conn := range connections {
			active = append(active, conn)
		}
		mu.Unlock()
		var closing sync.WaitGroup
		for _, conn := range active {
			closing.Go(func() { _ = conn.CloseNow() })
		}
		closing.Wait()
	}
}
