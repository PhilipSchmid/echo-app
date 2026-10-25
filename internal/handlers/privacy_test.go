package handlers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/proto"
	"github.com/coder/websocket"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func TestSuccessfulRequestsDoNotProduceInfoLogs(t *testing.T) {
	logs := captureDebugLogs(t)
	logrus.SetLevel(logrus.InfoLevel)
	cfg := &config.Config{PrintHeaders: true, MaxRequestSize: 1024}

	t.Run("HTTP", func(t *testing.T) {
		logs.Reset()
		req := httptest.NewRequest(http.MethodGet, "/inspect?token=query-private-marker", nil)
		req.Header.Set("User-Agent", "user-private-marker")
		req.Header.Set("Authorization", "credential-private-marker")
		response := httptest.NewRecorder()
		HTTPHandler(cfg, "HTTP")(response, req)
		require.Equal(t, http.StatusOK, response.Code)
		assert.Empty(t, logs.String(), "request diagnostics must require debug logging")
	})
	t.Run("gRPC", func(t *testing.T) {
		logs.Reset()
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("user-agent", "user-private-marker", "authorization", "credential-private-marker"))
		ctx = peer.NewContext(ctx, &peer.Peer{Addr: &mockAddr{addr: "192.0.2.1:54321"}})
		_, err := NewEchoServer(cfg).Echo(ctx, &proto.EchoRequest{})
		require.NoError(t, err)
		assert.Empty(t, logs.String(), "request diagnostics must require debug logging")
	})
	t.Run("TCP", func(t *testing.T) {
		logs.Reset()
		conn := new(MockTCPConn)
		conn.On("Write", mock.Anything).Return(1, nil).Once()
		conn.On("Close").Return(nil).Once()
		TCPHandler(context.Background(), conn, cfg)
		conn.AssertExpectations(t)
		assert.Empty(t, logs.String(), "request diagnostics must require debug logging")
	})
}

func TestHTTPLogsOmitBodiesQueriesAndCredentialHeaders(t *testing.T) {
	logs := captureDebugLogs(t)
	for _, major := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("HTTP%d", major), func(t *testing.T) {
			logs.Reset()
			req := httptest.NewRequest(http.MethodPost, "/inspect?token=query-private-marker", strings.NewReader("body-private-marker"))
			req.ProtoMajor = major
			req.Header.Set("Authorization", "auth-private-marker")
			req.Header.Set("Cookie", "cookie-private-marker")
			req.Header.Set("X-Password", "password-private-marker")
			response := httptest.NewRecorder()
			HTTPHandler(&config.Config{MaxRequestSize: 1024, PrintHeaders: true}, "HTTP")(response, req)
			require.Equal(t, http.StatusOK, response.Code)
			assert.Contains(t, logs.String(), "Request:", "assertions must inspect an active request logger")
			for _, value := range []string{"body-private-marker", "query-private-marker", "auth-private-marker", "cookie-private-marker", "password-private-marker"} {
				assert.NotContains(t, logs.String(), value)
			}
			assert.NotContains(t, response.Body.String(), "body-private-marker")
			assert.Contains(t, response.Body.String(), "query-private-marker", "diagnostics intentionally reflect the original URI; queries are not a safe place for secrets")
		})
	}
}

func TestWebSocketPayloadsAreEchoedWithoutApplicationLogging(t *testing.T) {
	logs := captureDebugLogs(t)
	server, _ := webSocketTestServer(t, 1024)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	const payload = "websocket-private-marker"
	require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(payload)))
	_, received, err := conn.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, payload, string(received))
	assert.NotContains(t, logs.String(), payload)
}

type privatePanicBody struct{ value string }

func (b privatePanicBody) Read([]byte) (int, error) { panic(b.value) }
func (privatePanicBody) Close() error               { return nil }

type privateErrorWriter struct {
	*httptest.ResponseRecorder
	err error
}

func (w privateErrorWriter) Write([]byte) (int, error) { return 0, w.err }

type privatePanicAddr struct{ value string }

func (privatePanicAddr) Network() string  { return "tcp" }
func (a privatePanicAddr) String() string { panic(a.value) }

func TestHandlerPanicLogsNeverIncludeRecoveredValues(t *testing.T) {
	for _, level := range []logrus.Level{logrus.InfoLevel, logrus.DebugLevel} {
		t.Run(level.String(), func(t *testing.T) {
			logs := captureDebugLogs(t)
			logrus.SetLevel(level)
			const secret = "panic-payload-private-marker"
			cfg := &config.Config{MaxRequestSize: 1024}
			t.Run("HTTP", func(t *testing.T) {
				logs.Reset()
				req := httptest.NewRequest(http.MethodPost, "/", nil)
				req.Body = privatePanicBody{secret}
				writer := privateErrorWriter{httptest.NewRecorder(), errors.New("write-error-private-marker")}
				HTTPHandler(cfg, "HTTP")(writer, req)
				assert.Equal(t, http.StatusInternalServerError, writer.Code)
				assert.Contains(t, logs.String(), "Recovered from panic")
				assert.NotContains(t, logs.String(), secret)
				assert.NotContains(t, logs.String(), "write-error-private-marker")
			})
			t.Run("TCP", func(t *testing.T) {
				logs.Reset()
				conn := &MockConn{remoteAddr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 54321}}
				conn.On("Write", mock.Anything).Panic(secret).Once()
				conn.On("Close").Return(nil).Once()
				TCPHandler(context.Background(), conn, cfg)
				conn.AssertExpectations(t)
				assert.Contains(t, logs.String(), "Recovered from panic")
				assert.NotContains(t, logs.String(), secret)
			})
			t.Run("gRPC", func(t *testing.T) {
				logs.Reset()
				ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: privatePanicAddr{secret}})
				_, err := NewEchoServer(cfg).Echo(ctx, &proto.EchoRequest{})
				assert.Equal(t, codes.Internal, status.Code(err))
				assert.Contains(t, logs.String(), "Recovered from panic")
				assert.NotContains(t, logs.String(), secret)
			})
		})
	}
}

func TestClientWriteAndCloseFailuresStayPrivateAndDebugOnly(t *testing.T) {
	for _, level := range []logrus.Level{logrus.InfoLevel, logrus.DebugLevel} {
		t.Run(level.String(), func(t *testing.T) {
			logs := captureDebugLogs(t)
			logrus.SetLevel(level)
			cfg := &config.Config{MaxRequestSize: 1024}
			const secret = "client-error-private-marker"
			writer := privateErrorWriter{httptest.NewRecorder(), errors.New(secret)}
			req := httptest.NewRequest(http.MethodPost, "/private-request-path", strings.NewReader("body-private-marker"))
			HTTPHandler(cfg, "HTTP")(writer, req)
			conn := &MockConn{remoteAddr: &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 54321}}
			conn.On("Write", mock.Anything).Return(0, errors.New(secret)).Once()
			conn.On("Close").Return(errors.New(secret)).Once()
			TCPHandler(context.Background(), conn, cfg)
			conn.AssertExpectations(t)
			assert.NotContains(t, logs.String(), secret)
			assert.NotContains(t, logs.String(), "body-private-marker")
			if level == logrus.InfoLevel {
				assert.Empty(t, logs.String(), "client disconnects must not emit access data or error-level noise")
			} else {
				assert.Contains(t, logs.String(), "Failed to write response")
				assert.Contains(t, logs.String(), "Failed to close TCP connection")
			}
		})
	}
}
