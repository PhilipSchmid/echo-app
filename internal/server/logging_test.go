package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func TestMain(m *testing.M) {
	ConfigureProtocolLogging()
	os.Exit(m.Run())
}

func captureProtocolLogs(t *testing.T, level logrus.Level) *logtest.Hook {
	t.Helper()
	logger := logrus.StandardLogger()
	previousLevel, previousOutput := logger.GetLevel(), logger.Out
	hook := logtest.NewGlobal()
	logger.SetLevel(level)
	logger.SetOutput(io.Discard)
	t.Cleanup(func() {
		logger.SetLevel(previousLevel)
		logger.SetOutput(previousOutput)
		hook.Reset()
	})
	return hook
}

func assertPrivateDiagnostics(t *testing.T, entries []*logrus.Entry) {
	t.Helper()
	for _, entry := range entries {
		for _, secret := range []string{"private-marker", "198.51.100.23", "Authorization", "request-private.example", "x-private-bin"} {
			assert.NotContains(t, entry.Message+fmt.Sprint(entry.Data), secret)
		}
	}
}

func TestProtocolDiagnosticsHavePrivateLoggers(t *testing.T) {
	cfg := &config.Config{HTTPPort: "8080", TLSPort: "8443", MaxRequestSize: 65536}
	for _, useTLS := range []bool{false, true} {
		server := NewHTTPServer(cfg, useTLS, nil)
		t.Cleanup(server.closeWebSockets)
		require.NotNil(t, server.server.ErrorLog, "HTTP diagnostics must not use the default raw stderr logger")
	}
	require.NotNil(t, NewQUICServer(cfg, nil).server.Logger, "HTTP/3 diagnostics must not log request attributes or raw panic values")
}

func TestProtocolDiagnosticSeverityAndRedaction(t *testing.T) {
	for _, level := range []logrus.Level{logrus.InfoLevel, logrus.DebugLevel} {
		t.Run(level.String(), func(t *testing.T) {
			hook := captureProtocolLogs(t, level)
			logger := httpErrorLog("TLS")
			logger.Print("http: TLS handshake error from 198.51.100.23:1234: private-marker")
			grpcDiagnostics{}.Warningf("Failed to decode metadata header %q: %s", "x-private-bin", "private-marker")
			grpcDiagnostics{}.Info("request-private.example private-marker")
			h3 := slog.New(http3Diagnostics{}).With("Authorization", "private-marker").WithGroup("private-marker")
			h3.Debug("handling request", "uri", "/?private-marker")
			h3.Debug("handling connection failed", "error", errors.New("private-marker"))
			if level == logrus.InfoLevel {
				assert.Empty(t, hook.AllEntries(), "client misuse must not produce default-level log spam")
			} else {
				require.Len(t, hook.AllEntries(), 3)
				for _, entry := range hook.AllEntries() {
					assert.Equal(t, logrus.DebugLevel, entry.Level)
				}
			}
			assertPrivateDiagnostics(t, hook.AllEntries())
			hook.Reset()
			logger.Print("http: panic serving 198.51.100.23:1234: private-marker\nAuthorization: private-marker")
			h3.Error("http3: panic serving", "arg", "private-marker", "trace", "private-marker")
			grpcDiagnostics{}.Errorf("Failed to marshal RPC status: private-marker")
			require.Len(t, hook.AllEntries(), 3)
			for _, entry := range hook.AllEntries() {
				assert.Equal(t, logrus.ErrorLevel, entry.Level)
			}
			assertPrivateDiagnostics(t, hook.AllEntries())
			hook.Reset()
			logger.Print("http: Accept error: private-marker; retrying in 5ms")
			log.Print("failed to sufficiently increase receive buffer size: private-marker")
			require.Len(t, hook.AllEntries(), 2)
			for _, entry := range hook.AllEntries() {
				assert.Equal(t, logrus.WarnLevel, entry.Level)
			}
			assertPrivateDiagnostics(t, hook.AllEntries())
		})
	}
}

func TestHTTPPanicLogExcludesPayloadAndPeer(t *testing.T) {
	hook := captureProtocolLogs(t, logrus.DebugLevel)
	server := NewHTTPServer(&config.Config{HTTPPort: "8080", MaxRequestSize: 65536}, false, nil)
	t.Cleanup(server.closeWebSockets)
	transport := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("private-marker") }))
	transport.Config.ErrorLog = server.server.ErrorLog
	transport.Start()
	t.Cleanup(transport.Close)
	response, err := transport.Client().Get(transport.URL + "/?private-marker")
	if response != nil {
		_ = response.Body.Close()
	}
	require.Error(t, err)
	require.Eventually(t, func() bool { return len(hook.AllEntries()) > 0 }, time.Second, time.Millisecond)
	entries := hook.AllEntries()
	require.Len(t, entries, 1)
	assert.Equal(t, logrus.ErrorLevel, entries[0].Level)
	assert.Equal(t, "HTTP handler panicked", entries[0].Message)
	assertPrivateDiagnostics(t, entries)
}

func TestHTTP3InvalidResponseLengthIsAnInternalError(t *testing.T) {
	hook := captureProtocolLogs(t, logrus.DebugLevel)
	logger := slog.New(http3Diagnostics{})
	logger.Error("Malformed Content-Length", "value", "private-marker")
	entries := hook.AllEntries()
	require.Len(t, entries, 1)
	assert.Equal(t, logrus.ErrorLevel, entries[0].Level)
	assert.Equal(t, "HTTP/3 handler returned an invalid Content-Length", entries[0].Message)
	assertPrivateDiagnostics(t, entries)
}

func TestGRPCFatalDiagnosticsTerminate(t *testing.T) {
	if method := os.Getenv("ECHO_TEST_GRPC_FATAL_METHOD"); method != "" {
		logrus.SetOutput(os.Stdout)
		logrus.SetFormatter(&logrus.JSONFormatter{})
		switch method {
		case "Fatal":
			grpcDiagnostics{}.Fatal("private-marker")
		case "Fatalln":
			grpcDiagnostics{}.Fatalln("private-marker")
		case "Fatalf":
			grpcDiagnostics{}.Fatalf("private-marker: %s", "private-marker")
		}
		t.Fatal("fatal logger returned without terminating the process")
	}
	for _, method := range []string{"Fatal", "Fatalln", "Fatalf"} {
		t.Run(method, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGRPCFatalDiagnosticsTerminate$")
			command.Env = append(os.Environ(), "ECHO_TEST_GRPC_FATAL_METHOD="+method)
			output, err := command.CombinedOutput()
			var exit *exec.ExitError
			require.ErrorAs(t, err, &exit)
			assert.Equal(t, 1, exit.ExitCode())
			assert.Contains(t, string(output), `"level":"fatal"`)
			assert.Contains(t, string(output), `"msg":"gRPC fatal error"`)
			assert.NotContains(t, string(output), "private-marker")
			assert.NotContains(t, string(output), "fatal logger returned")
		})
	}
}

func TestMalformedGRPCMetadataDoesNotReachLogs(t *testing.T) {
	hook := captureProtocolLogs(t, logrus.DebugLevel)
	server := NewGRPCServer(&config.Config{GRPCPort: "0", MaxRequestSize: 65536, MaxResponseSize: 65536}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- server.Start(ctx) }()
	select {
	case <-server.Ready():
	case err := <-result:
		t.Fatalf("start gRPC: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("gRPC listener did not become ready")
	}
	t.Cleanup(func() {
		shutdown, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		require.NoError(t, server.Shutdown(shutdown))
		require.NoError(t, <-result)
	})
	port := server.listener.Addr().(*net.TCPAddr).Port
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = io.WriteString(conn, http2.ClientPreface)
	require.NoError(t, err)
	framer := http2.NewFramer(conn, conn)
	require.NoError(t, framer.WriteSettings())
	var block bytes.Buffer
	encoder := hpack.NewEncoder(&block)
	for _, field := range []hpack.HeaderField{
		{Name: ":method", Value: "POST"}, {Name: ":scheme", Value: "http"},
		{Name: ":path", Value: "/echo.EchoService/Echo"}, {Name: ":authority", Value: "request-private.example"},
		{Name: "content-type", Value: "application/grpc"}, {Name: "te", Value: "trailers"},
		{Name: "x-private-bin", Value: "private-marker%%"},
	} {
		require.NoError(t, encoder.WriteField(field))
	}
	require.NoError(t, framer.WriteHeaders(http2.HeadersFrameParam{StreamID: 1, BlockFragment: block.Bytes(), EndHeaders: true, EndStream: true}))
	require.Eventually(t, func() bool {
		for _, entry := range hook.AllEntries() {
			if entry.Message == "gRPC transport warning" {
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond)
	assertPrivateDiagnostics(t, hook.AllEntries())
}

type deadlineFailureConn struct {
	net.Conn
	closed, wrote bool
}

func (*deadlineFailureConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("198.51.100.23"), Port: 1234}
}
func (*deadlineFailureConn) SetDeadline(time.Time) error      { return errors.New("private-marker") }
func (c *deadlineFailureConn) Close() error                   { c.closed = true; return nil }
func (c *deadlineFailureConn) Write(data []byte) (int, error) { c.wrote = true; return len(data), nil }

func TestTCPDeadlineFailureClosesWithoutLoggingPeer(t *testing.T) {
	hook := captureProtocolLogs(t, logrus.DebugLevel)
	server := NewTCPServer(&config.Config{MaxResponseSize: 65536})
	server.ctx = context.Background()
	server.wg.Add(1)
	server.activeConns = 1
	conn := &deadlineFailureConn{}
	server.handleConnection(conn)
	assert.True(t, conn.closed)
	assert.False(t, conn.wrote, "a failed deadline must not leave an unbounded connection running")
	require.Len(t, hook.AllEntries(), 1)
	assert.Equal(t, logrus.DebugLevel, hook.AllEntries()[0].Level)
	assertPrivateDiagnostics(t, hook.AllEntries())
}
