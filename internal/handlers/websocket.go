package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/coder/websocket"
)

// NewWebSocketHandler returns bounded message echo and a shutdown hook for hijacked connections.
func NewWebSocketHandler(cfg *config.Config, guards ...func() *protection.Guard) (http.Handler, func()) {
	var mu sync.Mutex
	closed := false
	connections := make(map[*websocket.Conn]struct{})
	recordFailure := func(err error, operation string) {
		mu.Lock()
		stopped := closed
		mu.Unlock()
		code := websocket.CloseStatus(err)
		if stopped || code == websocket.StatusNormalClosure || code == websocket.StatusGoingAway {
			return
		}
		reason := operation + "_error"
		if errors.Is(err, context.DeadlineExceeded) {
			reason = operation + "_timeout"
		} else if errors.Is(err, websocket.ErrMessageTooBig) || code == websocket.StatusMessageTooBig {
			reason = "message_too_large"
		}
		metrics.RecordError("WebSocket", reason)
	}
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
		conn.SetReadLimit(min(cfg.MaxRequestSize, responseLimit(cfg)))
		var guard *protection.Guard
		if len(guards) > 0 {
			guard = guards[0]()
		}
		sessionCtx := context.Background()
		if guard != nil {
			var cancel context.CancelFunc
			sessionCtx, cancel = context.WithTimeout(sessionCtx, 2*time.Minute)
			defer cancel()
		}
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
		messages, payloadBytes := 0, 0
		for {
			ctx, cancel := context.WithTimeout(sessionCtx, 30*time.Second)
			messageType, data, err := conn.Read(ctx)
			cancel()
			if err != nil {
				recordFailure(err, "read")
				return
			}
			if guard != nil {
				messages++
				payloadBytes += len(data)
				if messages > 60 || payloadBytes > 1<<20 {
					metrics.RecordRejection("WebSocket", "session_limit")
					_ = conn.Close(websocket.StatusPolicyViolation, "Session allowance reached; reconnect to continue")
					return
				}
				if err := guard.AllowOperation(r.RemoteAddr); err != nil {
					metrics.RecordRejection("WebSocket", err.Error())
					_ = conn.Close(websocket.StatusTryAgainLater, "Message rate limit reached")
					return
				}
			}
			kind := "text"
			if messageType == websocket.MessageBinary {
				kind = "binary"
			}
			metrics.RecordWebSocketMessage("received", kind, len(data))
			if messageType == websocket.MessageText && !utf8.Valid(data) {
				metrics.RecordError("WebSocket", "invalid_utf8")
				_ = conn.Close(websocket.StatusInvalidFramePayloadData, "invalid UTF-8")
				return
			}
			ctx, cancel = context.WithTimeout(sessionCtx, 30*time.Second)
			err = conn.Write(ctx, messageType, data)
			cancel()
			if err != nil {
				recordFailure(err, "write")
				return
			}
			metrics.RecordWebSocketMessage("sent", kind, len(data))
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
