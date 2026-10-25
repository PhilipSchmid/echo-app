package handlers

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/coder/websocket"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPEarlyReturnsAndRequestedErrorsAreObserved(t *testing.T) {
	for _, tc := range []struct {
		name, url, body, outcome string
		cancel                   bool
	}{
		{"invalid response control", "/?status=invalid", "", "400", false},
		{"oversized body", "/", "12345", "413", false},
		{"requested failure", "/?status=503", "", "503", false},
		{"canceled delay", "/?delay=250ms", "", "canceled", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := metrics.RequestsTotal.WithLabelValues("HTTP", "POST", "/")
			responses := metrics.HTTPResponsesTotal.WithLabelValues("HTTP", "/", tc.outcome)
			beforeRequests, beforeResponses := testutil.ToFloat64(requests), testutil.ToFloat64(responses)
			request := httptest.NewRequest(http.MethodPost, tc.url, strings.NewReader(tc.body))
			if tc.cancel {
				ctx, cancel := context.WithCancel(request.Context())
				cancel()
				request = request.WithContext(ctx)
			}
			handler := metrics.InstrumentHTTP("HTTP", HTTPHandler(&config.Config{MaxRequestSize: 4, ResponseControls: true}, "HTTP"))
			handler.ServeHTTP(httptest.NewRecorder(), request)
			assert.Equal(t, beforeRequests+1, testutil.ToFloat64(requests))
			assert.Equal(t, beforeResponses+1, testutil.ToFloat64(responses))
		})
	}
}

func TestWebSocketMetricsCountMessagesAndPreserveUpgrade(t *testing.T) {
	active := metrics.ActiveConnections.WithLabelValues("WebSocket")
	require.Eventually(t, func() bool { return testutil.ToFloat64(active) == 0 }, time.Second, time.Millisecond)
	handler, closeConnections := NewWebSocketHandler(&config.Config{MaxRequestSize: 1024})
	server := httptest.NewServer(metrics.InstrumentHTTP("HTTP", handler))
	t.Cleanup(func() { closeConnections(); server.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	assert.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
	for _, kind := range []websocket.MessageType{websocket.MessageText, websocket.MessageBinary} {
		label := "text"
		if kind == websocket.MessageBinary {
			label = "binary"
		}
		received := metrics.WebSocketMessagesTotal.WithLabelValues("received", label)
		sent := metrics.WebSocketMessagesTotal.WithLabelValues("sent", label)
		receivedBytes := metrics.WebSocketMessageBytesTotal.WithLabelValues("received", label)
		sentBytes := metrics.WebSocketMessageBytesTotal.WithLabelValues("sent", label)
		beforeReceived, beforeSent := testutil.ToFloat64(received), testutil.ToFloat64(sent)
		beforeReceivedBytes, beforeSentBytes := testutil.ToFloat64(receivedBytes), testutil.ToFloat64(sentBytes)
		payload := []byte("echo without labels")
		require.NoError(t, conn.Write(ctx, kind, payload))
		gotKind, got, err := conn.Read(ctx)
		require.NoError(t, err)
		assert.Equal(t, kind, gotKind)
		assert.Equal(t, payload, got)
		require.Eventually(t, func() bool { return testutil.ToFloat64(sent) == beforeSent+1 }, time.Second, time.Millisecond)
		assert.Equal(t, beforeReceived+1, testutil.ToFloat64(received))
		assert.Equal(t, beforeReceivedBytes+float64(len(payload)), testutil.ToFloat64(receivedBytes))
		assert.Equal(t, beforeSentBytes+float64(len(payload)), testutil.ToFloat64(sentBytes))
	}
	readErrors := metrics.ErrorsTotal.WithLabelValues("WebSocket", "read_error")
	beforeErrors := testutil.ToFloat64(readErrors)
	upgrades := metrics.HTTPResponsesTotal.WithLabelValues("HTTP", "/ws", "101")
	beforeUpgrades := testutil.ToFloat64(upgrades)
	bytes := metrics.HTTPResponseBytesTotal.WithLabelValues("HTTP")
	beforeBytes := testutil.ToFloat64(bytes)
	require.NoError(t, conn.Close(websocket.StatusNormalClosure, "done"))
	require.Eventually(t, func() bool {
		return testutil.ToFloat64(upgrades) == beforeUpgrades+1 && testutil.ToFloat64(active) == 0
	}, time.Second, time.Millisecond)
	assert.Equal(t, beforeErrors, testutil.ToFloat64(readErrors), "normal peer close is not a server error")
	assert.Equal(t, beforeBytes, testutil.ToFloat64(bytes), "WebSocket frame bytes are separate from HTTP body bytes")
}

func TestCanceledTCPConnectionIsClosedAndObserved(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = client.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	active := metrics.ActiveConnections.WithLabelValues("TCP")
	total := metrics.ConnectionsTotal.WithLabelValues("TCP")
	requests := metrics.RequestsTotal.WithLabelValues("TCP", "connection", "")
	beforeActive, beforeTotal, beforeRequests := testutil.ToFloat64(active), testutil.ToFloat64(total), testutil.ToFloat64(requests)
	TCPHandler(ctx, server, &config.Config{})
	_, err := client.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
	assert.Equal(t, beforeActive, testutil.ToFloat64(active))
	assert.Equal(t, beforeTotal+1, testutil.ToFloat64(total))
	assert.Equal(t, beforeRequests+1, testutil.ToFloat64(requests))
}
