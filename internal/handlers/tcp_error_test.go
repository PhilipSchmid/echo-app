package handlers

import (
	"net"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockConn is a mock implementation of net.Conn for testing
type MockConn struct {
	mock.Mock
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (m *MockConn) Read(b []byte) (n int, err error) {
	args := m.Called(b)
	return args.Int(0), args.Error(1)
}

func (m *MockConn) Write(b []byte) (n int, err error) {
	args := m.Called(b)
	return args.Int(0), args.Error(1)
}

func (m *MockConn) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockConn) LocalAddr() net.Addr {
	return m.localAddr
}

func (m *MockConn) RemoteAddr() net.Addr {
	return m.remoteAddr
}

func (m *MockConn) SetDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *MockConn) SetReadDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *MockConn) SetWriteDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func TestTCPHandler_WriteError(t *testing.T) {
	cfg := &config.Config{
		Message: "test-message",
		Node:    "test-node",
	}

	mockConn := new(MockConn)
	mockConn.remoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}

	// Mock Write to return an error
	mockConn.On("Write", mock.Anything).Return(0, assert.AnError)
	mockConn.On("Close").Return(nil)

	// Call the handler
	TCPHandler(mockConn, cfg)

	// Verify expectations
	mockConn.AssertExpectations(t)
}

func TestTCPHandler_CloseError(t *testing.T) {
	cfg := &config.Config{
		Message: "test-message",
		Node:    "test-node",
	}

	mockConn := new(MockConn)
	mockConn.remoteAddr = &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}

	// Mock successful write but close fails
	mockConn.On("Write", mock.Anything).Return(100, nil)
	mockConn.On("Close").Return(assert.AnError)

	// Call the handler
	TCPHandler(mockConn, cfg)

	// Verify expectations
	mockConn.AssertExpectations(t)
}

func TestTCPHandler_MalformedRemoteAddr(t *testing.T) {
	cfg := &config.Config{
		Message: "test-message",
		Node:    "test-node",
	}

	// Create a mock connection with a custom address type
	mockConn := new(MockConn)
	mockConn.remoteAddr = &mockAddr{addr: "invalid-address-format"}

	// Mock successful operations
	mockConn.On("Write", mock.Anything).Return(100, nil)
	mockConn.On("Close").Return(nil)

	// Call the handler - should not panic
	TCPHandler(mockConn, cfg)

	// Verify expectations
	mockConn.AssertExpectations(t)
}

// mockAddr is a custom net.Addr implementation for testing
type mockAddr struct {
	addr string
}

func (m *mockAddr) Network() string {
	return "tcp"
}

func (m *mockAddr) String() string {
	return m.addr
}
