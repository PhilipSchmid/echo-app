package handlers

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func protectedWebSocket(t *testing.T, cfg *config.Config, requestRate float64) (*websocket.Conn, context.Context) {
	t.Helper()
	guard, err := protection.NewGuard(protection.Options{
		Enabled: true, RequestRate: requestRate,
		IngressRate: 1 << 20, EgressRate: 1 << 20, DailyEgress: 10 << 20,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, guard.Close()) })
	handler, closeConnections := NewWebSocketHandler(cfg, func() *protection.Guard { return guard })
	server := httptest.NewServer(handler)
	t.Cleanup(func() { closeConnections(); server.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	require.NoError(t, err)
	conn.SetReadLimit(1 << 20)
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn, ctx
}

func requireWebSocketEcho(t *testing.T, ctx context.Context, conn *websocket.Conn, payload []byte) {
	t.Helper()
	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, payload))
	kind, echoed, err := conn.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageBinary, kind)
	assert.Equal(t, payload, echoed)
}

func TestWebSocketProtectionLimitsEveryMessage(t *testing.T) {
	conn, ctx := protectedWebSocket(t, &config.Config{MaxRequestSize: 1024, MaxResponseSize: 1024}, 0.001)
	requireWebSocketEcho(t, ctx, conn, []byte("first"))
	require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte("second")))
	_, _, err := conn.Read(ctx)
	require.Error(t, err)
	assert.Equal(t, websocket.StatusTryAgainLater, websocket.CloseStatus(err))
}

func TestWebSocketProtectionMessageCount(t *testing.T) {
	conn, ctx := protectedWebSocket(t, &config.Config{MaxRequestSize: 1024, MaxResponseSize: 1024}, 1000)
	for range 60 {
		requireWebSocketEcho(t, ctx, conn, []byte("echo"))
	}
	require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte("message 61")))
	_, _, err := conn.Read(ctx)
	require.Error(t, err)
	assert.Equal(t, websocket.StatusPolicyViolation, websocket.CloseStatus(err))
}

func TestWebSocketProtectionAggregatePayload(t *testing.T) {
	conn, ctx := protectedWebSocket(t, &config.Config{MaxRequestSize: 64 << 10, MaxResponseSize: 64 << 10}, 1000)
	payload := bytes.Repeat([]byte("x"), 64<<10)
	for range 16 {
		requireWebSocketEcho(t, ctx, conn, payload)
	}
	// Exactly 1 MiB is accepted; the next byte exceeds the session allowance.
	require.NoError(t, conn.Write(ctx, websocket.MessageBinary, []byte("x")))
	_, _, err := conn.Read(ctx)
	require.Error(t, err)
	assert.Equal(t, websocket.StatusPolicyViolation, websocket.CloseStatus(err))
}

func TestWebSocketProtectionUsesSmallerInputOrOutputCap(t *testing.T) {
	for _, tc := range []struct {
		name            string
		input, response int64
	}{
		{"input cap", 4, 8},
		{"response cap", 8, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, ctx := protectedWebSocket(t, &config.Config{MaxRequestSize: tc.input, MaxResponseSize: tc.response}, 1000)
			requireWebSocketEcho(t, ctx, conn, []byte("four"))
			require.NoError(t, conn.Write(ctx, websocket.MessageBinary, []byte("fives")))
			_, _, err := conn.Read(ctx)
			require.Error(t, err)
			assert.Equal(t, websocket.StatusMessageTooBig, websocket.CloseStatus(err))
		})
	}
}
