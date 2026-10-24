package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/proto"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestHTTPHandlerBodyLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		length int64
		want   int
	}{
		{"at limit", "1234", 4, http.StatusOK},
		{"known overflow", "12345", 5, http.StatusRequestEntityTooLarge},
		{"chunked overflow", "12345", -1, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			req.ContentLength = tc.length
			originalBody := req.Body
			w := httptest.NewRecorder()
			HTTPHandler(&config.Config{MaxRequestSize: 4}, "HTTP")(w, req)
			assert.Equal(t, tc.want, w.Code)
			assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			assert.Equal(t, originalBody, req.Body, "request must not retain nested body wrappers")
		})
	}
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("broken body") }
func (brokenBody) Close() error             { return nil }

func TestHTTPHandlerBodyReadError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = brokenBody{}
	req.ContentLength = -1
	w := httptest.NewRecorder()
	HTTPHandler(&config.Config{MaxRequestSize: 4}, "HTTP")(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func captureDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	logger := logrus.StandardLogger()
	oldOutput, oldLevel := logger.Out, logger.GetLevel()
	logs := &bytes.Buffer{}
	logger.SetOutput(logs)
	logger.SetLevel(logrus.DebugLevel)
	t.Cleanup(func() {
		logger.SetOutput(oldOutput)
		logger.SetLevel(oldLevel)
	})
	return logs
}

func TestHTTPDiagnosticHeaderRedaction(t *testing.T) {
	logs := captureDebugLogs(t)
	for _, unsafe := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodGet, "http://diagnostic.example/", nil)
		for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "X-Api-Key", "X-Access-Token", "X-Client-Secret"} {
			req.Header.Set(name, "credential-value")
		}
		req.Header.Set("X-Diagnostic", "visible-value")
		w := httptest.NewRecorder()
		logs.Reset()
		HTTPHandler(&config.Config{PrintHeaders: true, UnsafeHeaders: unsafe, MaxRequestSize: 4}, "HTTP")(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var response HTTPResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, []string{"visible-value"}, response.Headers["X-Diagnostic"])
		assert.Equal(t, "credential-value", req.Header.Get("Authorization"), "redaction must not mutate the request")
		assert.Contains(t, logs.String(), "diagnostic.example")
		if unsafe {
			assert.Contains(t, w.Body.String(), "credential-value")
			assert.Contains(t, logs.String(), "credential-value")
		} else {
			assert.NotContains(t, w.Body.String(), "credential-value")
			assert.NotContains(t, logs.String(), "credential-value")
			assert.Equal(t, []string{"[REDACTED]"}, response.Headers["Authorization"])
		}
	}
}

func TestGRPCMetadataRedaction(t *testing.T) {
	logs := captureDebugLogs(t)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "credential-value", "x-api-key", "api-key-value", "x-diagnostic", "visible-value"))
	_, err := NewEchoServer(&config.Config{}).Echo(ctx, &proto.EchoRequest{})
	require.NoError(t, err)
	assert.NotContains(t, logs.String(), "credential-value")
	assert.NotContains(t, logs.String(), "api-key-value")
	assert.Contains(t, logs.String(), "visible-value")
}

func TestGRPCPanicReturnsInternal(t *testing.T) {
	captureDebugLogs(t)
	response, err := NewEchoServer(nil).Echo(context.Background(), &proto.EchoRequest{})
	assert.Nil(t, response)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestHTTPHandlerDoesNotRetainBodyWrappers(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	originalBody := req.Body
	handler := HTTPHandler(&config.Config{MaxRequestSize: 4}, "HTTP")
	for range 10 {
		w := httptest.NewRecorder()
		handler(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, originalBody, req.Body)
	}
	_, err := io.Copy(io.Discard, req.Body)
	require.NoError(t, err)
}

type deadlineWriter struct {
	*httptest.ResponseRecorder
	readDeadline  time.Time
	writeDeadline time.Time
	readError     error
	writeError    error
}

func (w *deadlineWriter) SetReadDeadline(deadline time.Time) error {
	w.readDeadline = deadline
	return w.readError
}

func (w *deadlineWriter) SetWriteDeadline(deadline time.Time) error {
	w.writeDeadline = deadline
	return w.writeError
}

type checkingReader func([]byte) (int, error)

func (read checkingReader) Read(data []byte) (int, error) { return read(data) }

func TestHTTP3DeadlinesPrecedeBodyRead(t *testing.T) {
	w := &deadlineWriter{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.ProtoMajor = 3
	req.Body = io.NopCloser(checkingReader(func([]byte) (int, error) {
		assert.WithinDuration(t, time.Now().Add(30*time.Second), w.readDeadline, time.Second)
		assert.Equal(t, w.readDeadline, w.writeDeadline)
		return 0, io.EOF
	}))
	HTTPHandler(&config.Config{MaxRequestSize: 4}, "QUIC")(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHTTP3DeadlineErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		readError  error
		writeError error
		want       int
	}{
		{"unsupported recorder", http.ErrNotSupported, http.ErrNotSupported, http.StatusOK},
		{"read deadline failure", errors.New("read failed"), nil, http.StatusInternalServerError},
		{"write deadline failure", nil, errors.New("write failed"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &deadlineWriter{ResponseRecorder: httptest.NewRecorder(), readError: tc.readError, writeError: tc.writeError}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.ProtoMajor = 3
			HTTPHandler(&config.Config{MaxRequestSize: 4}, "QUIC")(w, req)
			assert.Equal(t, tc.want, w.Code)
		})
	}
}

func TestHTTP1KeepsServerDeadlines(t *testing.T) {
	w := &deadlineWriter{ResponseRecorder: httptest.NewRecorder()}
	HTTPHandler(&config.Config{MaxRequestSize: 4}, "HTTP")(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, w.readDeadline.IsZero())
	assert.True(t, w.writeDeadline.IsZero())
}
