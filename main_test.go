package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	pb "echo-app/proto"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/peer"
)

func TestEcho(t *testing.T) {
	// Suppress log output
	logrus.SetOutput(io.Discard)

	message := "Hello, World!"
	node := "test-node"
	server := &EchoServer{
		messagePtr: &message,
		nodePtr:    &node,
	}

	// Create a mock gRPC context with peer information
	clientIP := "127.0.0.1"
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP(clientIP),
			Port: 12345,
		},
	})

	req := &pb.EchoRequest{}
	resp, err := server.Echo(ctx, req)
	if err != nil {
		t.Fatalf("Echo() error = %v", err)
	}

	if resp.Message != message {
		t.Errorf("Echo() got = %v, want %v", resp.Message, message)
	}

	if resp.Node != node {
		t.Errorf("Echo() got = %v, want %v", resp.Node, node)
	}

	if resp.SourceIp != clientIP {
		t.Errorf("Echo() got = %v, want %v", resp.SourceIp, clientIP)
	}
}

func TestGenerateSelfSignedCert(t *testing.T) {
	// Suppress log output
	logrus.SetOutput(io.Discard)

	cert, err := generateSelfSignedCert()
	if err != nil {
		t.Fatalf("generateSelfSignedCert() error = %v", err)
	}

	// Convert the DER format to PEM format
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	if certPEM == nil {
		t.Fatalf("Failed to encode certificate to PEM")
	}

	// Verify the certificate
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatalf("Failed to parse certificate PEM")
	}
	_, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse x509 certificate: %v", err)
	}
}

func TestHTTPHandler(t *testing.T) {
	// Suppress log output
	logrus.SetOutput(io.Discard)

	message := "Hello, World!"
	node := "test-node"
	printHeaders := true
	handler := handleHTTPConnection(&message, &node, printHeaders, "HTTP")

	req := httptest.NewRequest("GET", "http://localhost:8080", nil)
	req.Header.Set("User-Agent", "test-agent")
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("HTTP handler returned wrong status code: got %v want %v", resp.StatusCode, http.StatusOK)
	}

	var response Response
	err := json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Message == nil || *response.Message != message {
		t.Errorf("HTTP handler returned wrong message: got %v want %v", response.Message, message)
	}

	if response.Node == nil || *response.Node != node {
		t.Errorf("HTTP handler returned wrong node: got %v want %v", response.Node, node)
	}

	if response.Endpoint != "HTTP" {
		t.Errorf("HTTP handler returned wrong endpoint: got %v want %v", response.Endpoint, "HTTP")
	}
}

func TestTCPHandler(t *testing.T) {
	// Suppress log output
	logrus.SetOutput(io.Discard)

	message := "Hello, World!"
	node := "test-node"
	endpoint := "TCP"

	// Start a TCP server
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to start TCP server: %v", err)
	}
	defer listener.Close()

	errChan := make(chan error, 1)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errChan <- err
			return
		}
		defer conn.Close()
		handleTCPConnection(conn, &message, &node)
		errChan <- nil
	}()

	// Connect to the TCP server
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Failed to connect to TCP server: %v", err)
	}
	defer conn.Close()

	var response Response
	err = json.NewDecoder(conn).Decode(&response)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Message == nil || *response.Message != message {
		t.Errorf("TCP handler returned wrong message: got %v want %v", response.Message, message)
	}

	if response.Node == nil || *response.Node != node {
		t.Errorf("TCP handler returned wrong node: got %v want %v", response.Node, node)
	}

	if response.Endpoint != endpoint {
		t.Errorf("TCP handler returned wrong endpoint: got %v want %v", response.Endpoint, endpoint)
	}

	if err := <-errChan; err != nil {
		t.Fatalf("Error in TCP handler goroutine: %v", err)
	}
}

func TestGRPCHandler(t *testing.T) {
	// Suppress log output
	logrus.SetOutput(io.Discard)

	message := "Hello, World!"
	node := "test-node"
	server := &EchoServer{
		messagePtr: &message,
		nodePtr:    &node,
	}

	// Create a mock gRPC context with peer information
	clientIP := "127.0.0.1"
	ctx := peer.NewContext(context.Background(), &peer.Peer{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP(clientIP),
			Port: 12345,
		},
	})

	req := &pb.EchoRequest{}
	resp, err := server.Echo(ctx, req)
	if err != nil {
		t.Fatalf("Echo() error = %v", err)
	}

	if resp.Message != message {
		t.Errorf("Echo() got = %v, want %v", resp.Message, message)
	}

	if resp.Node != node {
		t.Errorf("Echo() got = %v, want %v", resp.Node, node)
	}

	if resp.SourceIp != clientIP {
		t.Errorf("Echo() got = %v, want %v", resp.SourceIp, clientIP)
	}
}

func TestGetValidPort(t *testing.T) {
	// Suppress log output
	logrus.SetOutput(io.Discard)

	tests := []struct {
		envVar      string
		envValue    string
		defaultPort string
		expected    string
	}{
		{"TEST_PORT", "1234", "8080", "1234"},
		{"TEST_PORT", "", "8080", "8080"},
		{"TEST_PORT", "invalid", "8080", "8080"},
		{"TEST_PORT", "70000", "8080", "8080"},
	}

	for _, tt := range tests {
		t.Run(tt.envVar, func(t *testing.T) {
			os.Setenv(tt.envVar, tt.envValue)
			defer os.Unsetenv(tt.envVar)

			port := getValidPort(tt.envVar, tt.defaultPort)
			if port != tt.expected {
				t.Errorf("getValidPort(%s, %s) = %s; want %s", tt.envVar, tt.defaultPort, port, tt.expected)
			}
		})
	}
}

func TestIsValidPort(t *testing.T) {
	tests := []struct {
		port     string
		expected bool
	}{
		{"1234", true},
		{"0", false},
		{"65535", true},
		{"65536", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.port, func(t *testing.T) {
			if isValidPort(tt.port) != tt.expected {
				t.Errorf("isValidPort(%s) = %v; want %v", tt.port, !tt.expected, tt.expected)
			}
		})
	}
}

func TestSetLogLevel(t *testing.T) {
	tests := []struct {
		envValue string
		expected logrus.Level
	}{
		{"debug", logrus.DebugLevel},
		{"info", logrus.InfoLevel},
		{"warn", logrus.WarnLevel},
		{"error", logrus.ErrorLevel},
		{"invalid", logrus.InfoLevel}, // Fallback to info level
		{"", logrus.InfoLevel},        // Default to info level
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("LOG_LEVEL", tt.envValue)
			defer os.Unsetenv("LOG_LEVEL")

			setLogLevel()
			if logrus.GetLevel() != tt.expected {
				t.Errorf("setLogLevel() = %v; want %v", logrus.GetLevel(), tt.expected)
			}
		})
	}
}

func TestGetMessagePtr(t *testing.T) {
	tests := []struct {
		envValue string
		expected *string
	}{
		{"Hello, World!", stringPtr("Hello, World!")},
		{"", nil},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("MESSAGE", tt.envValue)
			defer os.Unsetenv("MESSAGE")

			result := getMessagePtr()
			if (result == nil && tt.expected != nil) || (result != nil && tt.expected == nil) {
				t.Errorf("getMessagePtr() = %v; want %v", result, tt.expected)
			} else if result != nil && *result != *tt.expected {
				t.Errorf("getMessagePtr() = %v; want %v", *result, *tt.expected)
			}
		})
	}
}

func TestGetNodePtr(t *testing.T) {
	tests := []struct {
		envValue string
		expected *string
	}{
		{"test-node", stringPtr("test-node")},
		{"", nil},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("NODE", tt.envValue)
			defer os.Unsetenv("NODE")

			result := getNodePtr()
			if (result == nil && tt.expected != nil) || (result != nil && tt.expected == nil) {
				t.Errorf("getNodePtr() = %v; want %v", result, tt.expected)
			} else if result != nil && *result != *tt.expected {
				t.Errorf("getNodePtr() = %v; want %v", *result, *tt.expected)
			}
		})
	}
}

func TestGetPrintHeadersSetting(t *testing.T) {
	tests := []struct {
		envValue string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"TRUE", true},
		{"FALSE", false},
		{"invalid", DefaultPrintHeaders},
		{"", DefaultPrintHeaders},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("PRINT_HTTP_REQUEST_HEADERS", tt.envValue)
			defer os.Unsetenv("PRINT_HTTP_REQUEST_HEADERS")

			result := getPrintHeadersSetting()
			if result != tt.expected {
				t.Errorf("getPrintHeadersSetting() = %v; want %v", result, tt.expected)
			}
		})
	}
}

func TestGetTLSSetting(t *testing.T) {
	tests := []struct {
		envValue string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"TRUE", true},
		{"FALSE", false},
		{"invalid", DefaultTLS},
		{"", DefaultTLS},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("TLS", tt.envValue)
			defer os.Unsetenv("TLS")

			result := getTLSSetting()
			if result != tt.expected {
				t.Errorf("getTLSSetting() = %v; want %v", result, tt.expected)
			}
		})
	}
}

func TestGetTCPSetting(t *testing.T) {
	tests := []struct {
		envValue string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"TRUE", true},
		{"FALSE", false},
		{"invalid", DefaultTCP},
		{"", DefaultTCP},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("TCP", tt.envValue)
			defer os.Unsetenv("TCP")

			result := getTCPSetting()
			if result != tt.expected {
				t.Errorf("getTCPSetting() = %v; want %v", result, tt.expected)
			}
		})
	}
}

func TestGetGRPCSetting(t *testing.T) {
	tests := []struct {
		envValue string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"TRUE", true},
		{"FALSE", false},
		{"invalid", DefaultGRPC},
		{"", DefaultGRPC},
	}

	for _, tt := range tests {
		t.Run(tt.envValue, func(t *testing.T) {
			os.Setenv("GRPC", tt.envValue)
			defer os.Unsetenv("GRPC")

			result := getGRPCSetting()
			if result != tt.expected {
				t.Errorf("getGRPCSetting() = %v; want %v", result, tt.expected)
			}
		})
	}
}

// Helper function to create a string pointer
func stringPtr(s string) *string {
	return &s
}
