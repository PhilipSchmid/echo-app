package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockTCPConn is a mock implementation of net.Conn
type MockTCPConn struct {
	mock.Mock
}

// Write mocks the Write method of net.Conn
func (m *MockTCPConn) Write(b []byte) (int, error) {
	args := m.Called(b)
	return args.Int(0), args.Error(1)
}

// Close mocks the Close method of net.Conn
func (m *MockTCPConn) Close() error {
	args := m.Called()
	return args.Error(0)
}

// Minimal implementations for other net.Conn methods (required for the interface)
func (m *MockTCPConn) Read(b []byte) (int, error) {
	return 0, nil // Not used in this test
}

func (m *MockTCPConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}

func (m *MockTCPConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
}

func (m *MockTCPConn) SetDeadline(t time.Time) error {
	return nil // Not used in this test
}

func (m *MockTCPConn) SetReadDeadline(t time.Time) error {
	return nil // Not used in this test
}

func (m *MockTCPConn) SetWriteDeadline(t time.Time) error {
	return nil // Not used in this test
}

func TestTCPHandler(t *testing.T) {
	// Create a test configuration
	cfg := &config.Config{
		Message: "Test TCP",
		Node:    "Test Node",
	}

	// Create a mock connection
	mockConn := new(MockTCPConn)

	// Set expectations for the mock
	mockConn.On("Write", mock.Anything).Return(len("some data"), nil).Once()
	mockConn.On("Close").Return(nil).Once()

	// Call the handler with the mock connection
	ctx := context.Background()
	TCPHandler(ctx, mockConn, cfg)

	// Retrieve the data passed to Write
	args := mockConn.Calls[0].Arguments // First call should be Write
	writtenData := args.Get(0).([]byte)

	// Unmarshal the written data to verify its contents
	var response TCPResponse
	err := json.Unmarshal(writtenData, &response)
	assert.NoError(t, err, "Failed to unmarshal response")

	// Verify the response fields
	assert.Equal(t, "Test TCP", response.Message, "Message field mismatch")
	assert.Equal(t, "Test Node", response.Node, "Node field mismatch")
	assert.Equal(t, "TCP", response.Listener, "Listener field mismatch")
	assert.NotEmpty(t, response.Timestamp, "Timestamp should not be empty")
	// Optionally verify Hostname and SourceIP if your handler sets them
	assert.NotEmpty(t, response.Hostname, "Hostname should not be empty")
	assert.Equal(t, "127.0.0.1", response.SourceIP, "SourceIP mismatch")

	// Ensure all mock expectations were met
	mockConn.AssertExpectations(t)
}

func TestTCPHandlerCapsEncodedResponseBeforeWriting(t *testing.T) {
	cfg := &config.Config{Message: strings.Repeat("<&", 100)}
	conn := new(MockTCPConn)
	data, err := json.Marshal(TCPResponse{BaseResponse: NewBaseResponse(cfg, "TCP", conn.RemoteAddr().String())})
	require.NoError(t, err)
	for _, delta := range []int64{0, -1} {
		t.Run(fmt.Sprintf("limit_delta_%d", delta), func(t *testing.T) {
			cfg.MaxResponseSize = int64(len(data)) + delta
			conn := new(MockTCPConn)
			conn.On("Close").Return(nil).Once()
			if delta == 0 {
				conn.On("Write", mock.Anything).Return(len(data), nil).Once()
			}
			tooLarge := metrics.ErrorsTotal.WithLabelValues("TCP", "response_too_large")
			before := testutil.ToFloat64(tooLarge)
			TCPHandler(context.Background(), conn, cfg)
			conn.AssertExpectations(t)
			if delta < 0 {
				conn.AssertNotCalled(t, "Write", mock.Anything)
				require.Equal(t, before+1, testutil.ToFloat64(tooLarge))
			} else {
				require.Equal(t, before, testutil.ToFloat64(tooLarge))
			}
		})
	}
}
