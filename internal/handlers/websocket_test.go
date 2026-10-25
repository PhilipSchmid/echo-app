package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func webSocketTestServer(t *testing.T, limit int64) (*httptest.Server, func()) {
	t.Helper()
	handler, closeConnections := NewWebSocketHandler(&config.Config{MaxRequestSize: limit})
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		closeConnections()
		server.Close()
	})
	return server, closeConnections
}

func TestWebSocketEchoTypesAndFragments(t *testing.T) {
	server, _ := webSocketTestServer(t, 1024)
	for _, kind := range []websocket.MessageType{websocket.MessageText, websocket.MessageBinary} {
		t.Run(kind.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
			require.NoError(t, err)
			defer func() { _ = conn.CloseNow() }()
			assert.Empty(t, response.Header.Get("Sec-WebSocket-Extensions"))
			payload := []byte("first message")
			if kind == websocket.MessageBinary {
				payload = []byte{0, 255, 1, 254}
			}
			require.NoError(t, conn.Write(ctx, kind, payload))
			gotKind, got, err := conn.Read(ctx)
			require.NoError(t, err)
			assert.Equal(t, kind, gotKind)
			assert.Equal(t, payload, got)

			writer, err := conn.Writer(ctx, kind)
			require.NoError(t, err)
			_, err = writer.Write([]byte("fragment "))
			require.NoError(t, err)
			_, err = writer.Write([]byte("two"))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			gotKind, got, err = conn.Read(ctx)
			require.NoError(t, err)
			assert.Equal(t, kind, gotKind)
			assert.Equal(t, []byte("fragment two"), got)
		})
	}
}

func TestWebSocketOrigin(t *testing.T) {
	server, _ := webSocketTestServer(t, 1024)
	for _, tc := range []struct {
		origin  string
		allowed bool
	}{
		{"", true},
		{server.URL, true},
		{"http://attacker.example", false},
		{"https" + strings.TrimPrefix(server.URL, "http"), true},
		{"ftp" + strings.TrimPrefix(server.URL, "http"), false},
		{"null", false},
	} {
		t.Run(tc.origin, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{tc.origin}}})
			if tc.allowed {
				require.NoError(t, err)
				_ = conn.CloseNow()
			} else {
				require.Error(t, err)
				require.NotNil(t, response)
				defer func() { _ = response.Body.Close() }()
				assert.Equal(t, http.StatusForbidden, response.StatusCode)
			}
		})
	}
}

func TestWebSocketMessageLimits(t *testing.T) {
	for _, tc := range []struct {
		name       string
		limit      int64
		size       int
		fragmented bool
	}{
		{"configured limit", 4, 5, false},
		{"hard ceiling", 2 << 20, (1 << 20) + 1, false},
		{"fragment aggregate", 8, 5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := webSocketTestServer(t, tc.limit)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
			require.NoError(t, err)
			defer func() { _ = conn.CloseNow() }()
			payload := bytes.Repeat([]byte("x"), tc.size)
			if tc.fragmented {
				writer, err := conn.Writer(ctx, websocket.MessageText)
				require.NoError(t, err)
				_, err = writer.Write(payload)
				require.NoError(t, err)
				_, err = writer.Write(payload)
				require.NoError(t, err)
				_ = writer.Close()
			} else {
				_ = conn.Write(ctx, websocket.MessageBinary, payload)
			}
			_, _, err = conn.Read(ctx)
			require.Error(t, err)
			assert.Equal(t, websocket.StatusMessageTooBig, websocket.CloseStatus(err))
		})
	}
}

func TestWebSocketShutdownClosesConnections(t *testing.T) {
	server, closeConnections := webSocketTestServer(t, 1024)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var connections []*websocket.Conn
	for range 4 {
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
		require.NoError(t, err)
		defer func() { _ = conn.CloseNow() }()
		require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte("registered")))
		_, _, err = conn.Read(ctx)
		require.NoError(t, err)
		connections = append(connections, conn)
	}
	// Mix a peer entering the close handshake with connections waiting for messages.
	require.NoError(t, connections[0].Write(ctx, websocket.MessageText, []byte{255}))
	closing := make(chan error, 1)
	go func() {
		_, _, err := connections[0].Read(ctx)
		closing <- err
	}()
	closeConnections()
	closeConnections()
	require.Error(t, <-closing)
	for _, conn := range connections[1:] {
		_, _, err := conn.Read(ctx)
		require.Error(t, err)
	}
	response, err := http.Get(server.URL + "/ws")
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	assert.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
}

func TestWebSocketHandshakeRejectsBodiesAndOtherHTTPVersions(t *testing.T) {
	handler, closeConnections := NewWebSocketHandler(&config.Config{MaxRequestSize: 4})
	defer closeConnections()
	for _, tc := range []struct {
		name    string
		major   int
		minor   int
		length  int64
		chunked bool
		want    int
	}{
		{"HTTP2", 2, 0, 0, false, http.StatusUpgradeRequired},
		{"HTTP3", 3, 0, 0, false, http.StatusUpgradeRequired},
		{"HTTP1.0", 1, 0, 0, false, http.StatusUpgradeRequired},
		{"known body", 1, 1, 1, false, http.StatusBadRequest},
		{"unknown body", 1, 1, -1, true, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws", nil)
			req.ProtoMajor, req.ProtoMinor = tc.major, tc.minor
			req.ContentLength = tc.length
			if tc.chunked {
				req.TransferEncoding = []string{"chunked"}
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.Equal(t, tc.want, w.Code)
		})
	}
}

func TestWebSocketTextValidation(t *testing.T) {
	server, _ := webSocketTestServer(t, 1024)
	for _, tc := range []struct {
		name      string
		kind      websocket.MessageType
		fragments [][]byte
		invalid   bool
	}{
		{"invalid text", websocket.MessageText, [][]byte{{255}}, true},
		{"UTF-8 across fragments", websocket.MessageText, [][]byte{{0xe2}, {0x82}, {0xac}}, false},
		{"binary is not UTF-8", websocket.MessageBinary, [][]byte{{255}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
			require.NoError(t, err)
			defer func() { _ = conn.CloseNow() }()
			writer, err := conn.Writer(ctx, tc.kind)
			require.NoError(t, err)
			for _, fragment := range tc.fragments {
				_, err = writer.Write(fragment)
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())
			kind, payload, err := conn.Read(ctx)
			if tc.invalid {
				require.Error(t, err)
				assert.Equal(t, websocket.StatusInvalidFramePayloadData, websocket.CloseStatus(err))
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.kind, kind)
				assert.Equal(t, bytes.Join(tc.fragments, nil), payload)
			}
		})
	}
}
