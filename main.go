package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/quic-go/quic-go/http3"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"

	pb "echo-app/proto"
)

// Default port numbers
const (
	DefaultMessage = ""
	DefaultNode    = ""

	DefaultPrintHeaders = false
	DefaultTLS          = false
	DefaultTCP          = false
	DefaultGRPC         = false
	DefaultQUIC         = false

	DefaultLogLevel = log.InfoLevel

	DefaultHTTPPort = "8080"
	DefaultTLSPort  = "8443"
	DefaultTCPPort  = "9090"
	DefaultGRPCPort = "50051"
	DefaultQUICPort = "4433"
)

// Response is the struct for the JSON response
type Response struct {
	Timestamp    string              `json:"timestamp"`
	Message      *string             `json:"message,omitempty"`
	SourceIP     string              `json:"source_ip"`
	Hostname     string              `json:"hostname"`
	Listener     string              `json:"listener"`          // Field to include the listener name
	Node         *string             `json:"node,omitempty"`    // Optional field to include node name
	Headers      map[string][]string `json:"headers,omitempty"` // Optional field to include headers
	HTTPVersion  string              `json:"http_version,omitempty"`
	HTTPMethod   string              `json:"http_method,omitempty"`
	HTTPEndpoint string              `json:"http_endpoint,omitempty"`
	GRPCMethod   string              `json:"grpc_method,omitempty"`
}

// EchoServer is the gRPC server that implements the EchoService
type EchoServer struct {
	pb.UnimplementedEchoServiceServer
	messagePtr *string
	nodePtr    *string
}

// Echo handles the Echo gRPC request
func (s *EchoServer) Echo(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	// Get the current time in human-readable format with milliseconds
	timestamp := time.Now().Format("2006-01-02T15:04:05.999Z07:00")
	host, _ := os.Hostname()

	// Extract the client's IP address from the context
	var clientIP string
	if p, ok := peer.FromContext(ctx); ok {
		if addr, ok := p.Addr.(*net.TCPAddr); ok {
			clientIP = addr.IP.String()
		}
	}

	// Extract the gRPC method name from the context
	method, _ := grpc.Method(ctx)

	// Log the serving request with detailed information
	log.Infof("Serving gRPC request from %s via gRPC listener, method: %s", clientIP, method)

	// Create the response struct
	response := &pb.EchoResponse{
		Timestamp:  timestamp,
		Hostname:   host,
		Listener:   "gRPC",
		SourceIp:   clientIP,
		GrpcMethod: method,
	}

	// Optionally set the message if it's not nil
	if s.messagePtr != nil {
		response.Message = *s.messagePtr
	}

	// Optionally set the node name if it's not nil
	if s.nodePtr != nil {
		response.Node = *s.nodePtr
	}

	return response, nil
}

func main() {
	// Set up Viper
	viper.AutomaticEnv()
	viper.SetEnvPrefix("ECHO_APP")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// Define command line flags using pflag
	pflag.String("message", DefaultMessage, "Custom message to include in the response")
	pflag.String("node", DefaultNode, "Node name to include in the response")
	pflag.Bool("print-http-request-headers", DefaultPrintHeaders, "Include HTTP request headers in the response")
	pflag.Bool("tls", DefaultTLS, "Enable TLS (HTTPS) support")
	pflag.Bool("tcp", DefaultTCP, "Enable TCP listener")
	pflag.Bool("grpc", DefaultGRPC, "Enable gRPC listener")
	pflag.Bool("quic", DefaultQUIC, "Enable QUIC listener")
	pflag.String("port", DefaultHTTPPort, "Port for the HTTP server")
	pflag.String("tls-port", DefaultTLSPort, "Port for the TLS server")
	pflag.String("tcp-port", DefaultTCPPort, "Port for the TCP server")
	pflag.String("grpc-port", DefaultGRPCPort, "Port for the gRPC server")
	pflag.String("quic-port", DefaultQUICPort, "Port for the QUIC server")
	pflag.String("log-level", DefaultLogLevel.String(), "Logging level (debug, info, warn, error)")

	// Set custom usage function
	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "echo-app: A simple Go application that responds with a JSON payload containing various details.\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		pflag.PrintDefaults()
	}

	// Parse command line flags
	pflag.Parse()

	// Bind command line flags to Viper
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Fatalf("Failed to bind command line flags: %v", err)
	}

	// Set up logging
	setLogLevel()

	// Get configuration values
	messagePtr := getMessagePtr()
	nodePtr := getNodePtr()
	printHeaders := getPrintHeadersSetting()
	tlsEnabled := getTLSSetting()
	tcpEnabled := getTCPSetting()
	grpcEnabled := getGRPCSetting()
	quicEnabled := getQUICSetting()

	// Prepare the message log
	messageLog := "No MESSAGE environment variable set"
	if messagePtr != nil {
		messageLog = "MESSAGE is set to: " + *messagePtr
	}

	// Prepare the node log
	nodeLog := "No NODE environment variable set"
	if nodePtr != nil {
		nodeLog = "NODE environment variable set to: " + *nodePtr
	}

	// Print optional configs on multiple lines
	log.Debug("Server configuration:")
	log.Debugf("  %s", messageLog)
	log.Debugf("  %s", nodeLog)
	log.Debugf("  PRINT_HTTP_REQUEST_HEADERS is set to: %t", printHeaders)
	log.Debugf("  TLS is set to: %t", tlsEnabled)
	log.Debugf("  TCP is set to: %t", tcpEnabled)
	log.Debugf("  GRPC is set to: %t", grpcEnabled)
	log.Debugf("  QUIC is set to: %t", quicEnabled)

	// Use PORT environment variable, or default to DefaultHTTPPort
	port := getValidPort("port", DefaultHTTPPort)

	// Register handleHTTPConnection function to handle all requests
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleHTTPConnection(messagePtr, nodePtr, printHeaders, "HTTP")) // Pass message, node pointers, printHeaders, and listener name to the handleHTTPConnection function

	// Start the web server on port and accept requests
	go func() {
		listener, err := net.Listen("tcp", ":"+port)
		if err != nil {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
		defer listener.Close()

		log.Infof("HTTP server listening on port %s (%s)", port, getL4Protocol(listener))
		log.Fatal(http.Serve(listener, mux))
	}()

	if tlsEnabled {
		startTLSServer(messagePtr, nodePtr, printHeaders)
	}

	if tcpEnabled {
		startTCPServer(messagePtr, nodePtr)
	}

	if grpcEnabled {
		startGRPCServer(messagePtr, nodePtr)
	}

	if quicEnabled {
		startQUICServer(messagePtr, nodePtr, printHeaders)
	}

	// Handle OS signals
	handleSignals()

	// Block forever
	select {}
}

func startTLSServer(messagePtr, nodePtr *string, printHeaders bool) {
	// Use TLS_PORT environment variable, or default to DefaultTLSPort
	tlsPort := getValidPort("tls-port", DefaultTLSPort)

	// Generate in-memory TLS certificate pair
	cert, err := generateSelfSignedCert()
	if err != nil {
		log.Fatalf("Failed to generate self-signed certificate: %v", err)
	}

	// Start the HTTPS server on the specified TLS port
	go func() {
		listener, err := tls.Listen("tcp", ":"+tlsPort, &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
		if err != nil {
			log.Fatalf("Failed to start TLS server: %v", err)
		}
		defer listener.Close()

		log.Infof("TLS server listening on port %s (%s)", tlsPort, getL4Protocol(listener))
		server := &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handleHTTPConnection(messagePtr, nodePtr, printHeaders, "TLS")(w, r)
			}),
		}
		log.Fatal(server.Serve(listener))
	}()
}

func startTCPServer(messagePtr, nodePtr *string) {
	// Use TCP_PORT environment variable, or default to DefaultTCPPort
	tcpPort := getValidPort("tcp-port", DefaultTCPPort)

	// Start the TCP server on the specified TCP port
	go func() {
		listener, err := net.Listen("tcp", ":"+tcpPort)
		if err != nil {
			log.Fatalf("Failed to start TCP server: %v", err)
		}
		defer listener.Close()

		log.Infof("TCP server listening on port %s (%s)", tcpPort, getL4Protocol(listener))
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Errorf("Failed to accept TCP connection: %v", err)
				continue
			}
			go handleTCPConnection(conn, messagePtr, nodePtr)
		}
	}()
}

func startGRPCServer(messagePtr, nodePtr *string) {
	// Use GRPC_PORT environment variable, or default to DefaultGRPCPort
	grpcPort := getValidPort("grpc-port", DefaultGRPCPort)

	// Start the gRPC server on the specified gRPC port
	go func() {
		listener, err := net.Listen("tcp", ":"+grpcPort)
		if err != nil {
			log.Fatalf("Failed to start gRPC server: %v", err)
		}
		defer listener.Close()

		log.Infof("gRPC server listening on port %s (%s)", grpcPort, getL4Protocol(listener))
		grpcServer := grpc.NewServer()
		pb.RegisterEchoServiceServer(grpcServer, &EchoServer{messagePtr: messagePtr, nodePtr: nodePtr})
		reflection.Register(grpcServer)

		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("Failed to serve gRPC server: %v", err)
		}
	}()
}

func startQUICServer(messagePtr, nodePtr *string, printHeaders bool) {
	// Use QUIC_PORT environment variable, or default to DefaultQUICPort
	quicPort := getValidPort("quic-port", DefaultQUICPort)

	// Generate in-memory TLS certificate pair
	cert, err := generateSelfSignedCert()
	if err != nil {
		log.Fatalf("Failed to generate self-signed certificate: %v", err)
	}

	// Start the HTTP/3 server on the specified QUIC port
	go func() {
		server := &http3.Server{
			Addr: ":" + quicPort,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handleHTTPConnection(messagePtr, nodePtr, printHeaders, "QUIC")(w, r)
			}),
			TLSConfig: http3.ConfigureTLSConfig(&tls.Config{
				Certificates: []tls.Certificate{cert},
			}),
		}
		defer server.Close()

		log.Infof("QUIC server listening on port %s (UDP)", quicPort)
		log.Fatal(server.ListenAndServe())
	}()
}

// getL4Protocol determines the L4 protocol (TCP or UDP) from the listener.
func getL4Protocol(listener net.Listener) string {
	switch listener.Addr().Network() {
	case "tcp", "tcp4", "tcp6":
		return "TCP"
	case "udp", "udp4", "udp6":
		return "UDP"
	default:
		return "Unknown"
	}
}

// handleHTTPConnection returns a http.HandlerFunc that uses the provided message pointer, node pointer, printHeaders flag, and listener name.
func handleHTTPConnection(messagePtr *string, nodePtr *string, printHeaders bool, listener string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get the IP address without the port number
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			log.Errorf("Error getting remote address: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Log the serving request with detailed information in info log level, as serving those is the core functionality of the application.
		log.Infof("Serving request: %s %s from %s (User-Agent: %s) via %s listener, HTTP version: %s", r.Method, r.URL.Path, ip, r.UserAgent(), listener, r.Proto)
		host, _ := os.Hostname()

		// Get the current time in human-readable format with milliseconds
		timestamp := time.Now().Format("2006-01-02T15:04:05.999Z07:00")

		// Create the response struct with the timestamp as the first field
		response := Response{
			Timestamp:    timestamp,
			Message:      messagePtr,
			Hostname:     host,
			Listener:     listener,
			Node:         nodePtr,
			SourceIP:     ip,
			HTTPVersion:  r.Proto,
			HTTPMethod:   r.Method,
			HTTPEndpoint: r.URL.Path,
		}

		// Conditionally add headers if printHeaders is true
		if printHeaders {
			response.Headers = make(map[string][]string)
			for name, values := range r.Header {
				response.Headers[name] = values
			}
		}

		// Set the Content-Type header to application/json
		w.Header().Set("Content-Type", "application/json")

		// Encode the response struct to JSON and send it as the response
		err = json.NewEncoder(w).Encode(response)
		if err != nil {
			log.Errorf("Error encoding JSON response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}
}

// handleTCPConnection handles a TCP connection and sends the JSON response.
func handleTCPConnection(conn net.Conn, messagePtr *string, nodePtr *string) {
	defer conn.Close()

	// Get the IP address without the port number
	ip, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		log.Errorf("Error getting remote address: %v", err)
		return
	}

	// Log the serving request with detailed information
	log.Infof("Serving TCP request from %s via TCP listener", ip)
	host, _ := os.Hostname()

	// Get the current time in human-readable format with milliseconds
	timestamp := time.Now().Format("2006-01-02T15:04:05.999Z07:00")

	// Create the response struct with the timestamp as the first field
	response := Response{
		Timestamp: timestamp,
		Message:   messagePtr,
		Hostname:  host,
		Listener:  "TCP",
		Node:      nodePtr,
		SourceIP:  ip,
	}

	// Encode the response struct to JSON and send it as the response
	err = json.NewEncoder(conn).Encode(response)
	if err != nil {
		log.Errorf("Error encoding JSON response: %v", err)
	}
}

// getMessagePtr gets the MESSAGE environment variable and returns a pointer to it, or nil if it's not set or invalid.
func getMessagePtr() *string {
	message := viper.GetString("message")
	if message == "" {
		log.Debugf("No MESSAGE environment variable set. Falling back to default value: '%s'", DefaultMessage)
		return nil
	}
	return &message
}

// getNodePtr gets the NODE environment variable and returns a pointer to it, or nil if it's not set or invalid.
func getNodePtr() *string {
	node := viper.GetString("node")
	if node == "" {
		log.Debugf("No NODE environment variable set. Falling back to default value: '%s'", DefaultNode)
		return nil
	}
	return &node
}

// getPrintHeadersSetting checks the PRINT_HTTP_REQUEST_HEADERS environment variable.
func getPrintHeadersSetting() bool {
	return viper.GetBool("print-http-request-headers")
}

// getTLSSetting checks the TLS environment variable.
func getTLSSetting() bool {
	return viper.GetBool("tls")
}

// getTCPSetting checks the TCP environment variable.
func getTCPSetting() bool {
	return viper.GetBool("tcp")
}

// getGRPCSetting checks the GRPC environment variable.
func getGRPCSetting() bool {
	return viper.GetBool("grpc")
}

// getQUICSetting checks the QUIC environment variable.
func getQUICSetting() bool {
	return viper.GetBool("quic")
}

// setLogLevel sets the log level based on the LOG_LEVEL environment variable.
func setLogLevel() {
	logLevel := viper.GetString("log-level")
	if logLevel == "" {
		// Default log level should be "info"
		logLevel = DefaultLogLevel.String()
	}
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		log.Warnf("Invalid log level: %v. Falling back to '%s'.", err, DefaultLogLevel)
		level = DefaultLogLevel
	}
	log.SetLevel(level)
}

// getValidPort retrieves and validates the port from the environment variable, falling back to the default if invalid.
func getValidPort(envVar string, defaultPort string) string {
	port := viper.GetString(envVar)
	if port == "" {
		log.Debugf("No port for %s set. Falling back to default port: '%s'", envVar, defaultPort)
		return defaultPort
	}
	if !isValidPort(port) {
		log.Warnf("Invalid port for %s: %s. Falling back to default port: '%s'", envVar, port, defaultPort)
		return defaultPort
	}
	return port
}

// isValidPort checks if the given port is a valid port number.
func isValidPort(port string) bool {
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	return portNum > 0 && portNum <= 65535
}

// generateSelfSignedCert generates a self-signed TLS certificate.
func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"echo Inc."},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years validity

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return tls.X509KeyPair(certPEM, keyPEM)
}

// handleSignals sets up signal handling for SIGINT and SIGTSTP.
func handleSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTSTP)

	go func() {
		for sig := range sigChan {
			log.Infof("Received signal: %s. Terminating...", sig)
			os.Exit(0)
		}
	}()
}
