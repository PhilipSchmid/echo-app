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
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"

	pb "echo-app/proto"
)

// Response is the struct for the JSON response
type Response struct {
	Timestamp string              `json:"timestamp"`
	Message   *string             `json:"message,omitempty"`
	SourceIP  string              `json:"source_ip"`
	Hostname  string              `json:"hostname"`
	Endpoint  string              `json:"endpoint"`          // Field to include the endpoint name
	Node      *string             `json:"node,omitempty"`    // Optional field to include node name
	Headers   map[string][]string `json:"headers,omitempty"` // Optional field to include headers
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

	// Log the serving request with detailed information
	log.Printf("Serving gRPC request from %s via gRPC endpoint", clientIP)

	// Create the response struct
	response := &pb.EchoResponse{
		Timestamp: timestamp,
		Hostname:  host,
		Endpoint:  "gRPC",
		SourceIp:  clientIP,
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
	// Get MESSAGE, NODE, PRINT_HTTP_REQUEST_HEADERS, TLS, TCP, and GRPC environment variables
	messagePtr := getMessagePtr()
	nodePtr := getNodePtr()
	printHeaders := getPrintHeadersSetting()
	tlsEnabled := getTLSSetting()
	tcpEnabled := getTCPSetting()
	grpcEnabled := getGRPCSetting()

	// Prepare the message log
	messageLog := "No MESSAGE environment variable set"
	if messagePtr != nil {
		messageLog = "MESSAGE environment variable set to: " + *messagePtr
	}

	// Prepare the node log
	nodeLog := "No NODE environment variable set"
	if nodePtr != nil {
		nodeLog = "NODE environment variable set to: " + *nodePtr
	}

	// Print optional configs on multiple lines
	log.Println("Server configuration:")
	log.Printf("  %s\n", messageLog)
	log.Printf("  %s\n", nodeLog)
	if printHeaders {
		log.Println("  PRINT_HTTP_REQUEST_HEADERS is enabled")
	} else {
		log.Println("  PRINT_HTTP_REQUEST_HEADERS is disabled")
	}
	if tlsEnabled {
		log.Println("  TLS is enabled")
	} else {
		log.Println("  TLS is disabled")
	}
	if tcpEnabled {
		log.Println("  TCP is enabled")
	} else {
		log.Println("  TCP is disabled")
	}
	if grpcEnabled {
		log.Println("  gRPC is enabled")
	} else {
		log.Println("  gRPC is disabled")
	}

	// Register hello function to handle all requests
	mux := http.NewServeMux()
	mux.HandleFunc("/", hello(messagePtr, nodePtr, printHeaders, "HTTP")) // Pass message, node pointers, printHeaders, and endpoint name to the hello function

	// Use PORT environment variable, or default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start the web server on port and accept requests
	go func() {
		log.Printf("Server listening on port %s\n", port)
		log.Fatal(http.ListenAndServe(":"+port, mux))
	}()

	if tlsEnabled {
		// Use TLS_PORT environment variable, or default to 8443
		tlsPort := os.Getenv("TLS_PORT")
		if tlsPort == "" {
			tlsPort = "8443"
		}

		// Generate in-memory TLS certificate pair
		cert, err := generateSelfSignedCert()
		if err != nil {
			log.Fatalf("Failed to generate self-signed certificate: %v", err)
		}

		// Start the HTTPS server on the specified TLS port
		go func() {
			server := &http.Server{
				Addr: ":" + tlsPort,
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hello(messagePtr, nodePtr, printHeaders, "HTTPS")(w, r)
				}),
				TLSConfig: &tls.Config{
					Certificates: []tls.Certificate{cert},
				},
			}

			log.Printf("TLS server listening on port %s\n", tlsPort)
			log.Fatal(server.ListenAndServeTLS("", ""))
		}()
	}

	if tcpEnabled {
		// Use TCP_PORT environment variable, or default to 9090
		tcpPort := os.Getenv("TCP_PORT")
		if tcpPort == "" {
			tcpPort = "9090"
		}

		// Start the TCP server on the specified TCP port
		go func() {
			listener, err := net.Listen("tcp", ":"+tcpPort)
			if err != nil {
				log.Fatalf("Failed to start TCP server: %v", err)
			}
			defer listener.Close()

			log.Printf("TCP server listening on port %s\n", tcpPort)
			for {
				conn, err := listener.Accept()
				if err != nil {
					log.Printf("Failed to accept TCP connection: %v", err)
					continue
				}
				go handleTCPConnection(conn, messagePtr, nodePtr, "TCP")
			}
		}()
	}

	if grpcEnabled {
		// Use GRPC_PORT environment variable, or default to 50051
		grpcPort := os.Getenv("GRPC_PORT")
		if grpcPort == "" {
			grpcPort = "50051"
		}

		// Start the gRPC server on the specified gRPC port
		go func() {
			listener, err := net.Listen("tcp", ":"+grpcPort)
			if err != nil {
				log.Fatalf("Failed to start gRPC server: %v", err)
			}
			defer listener.Close()

			grpcServer := grpc.NewServer()
			pb.RegisterEchoServiceServer(grpcServer, &EchoServer{messagePtr: messagePtr, nodePtr: nodePtr})
			reflection.Register(grpcServer)

			log.Printf("gRPC server listening on port %s\n", grpcPort)
			if err := grpcServer.Serve(listener); err != nil {
				log.Fatalf("Failed to serve gRPC server: %v", err)
			}
		}()
	}

	// Block forever
	select {}
}

// hello returns a http.HandlerFunc that uses the provided message pointer, node pointer, printHeaders flag, and endpoint name.
func hello(messagePtr *string, nodePtr *string, printHeaders bool, endpoint string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get the IP address without the port number
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			log.Printf("Error getting remote address: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Log the serving request with detailed information
		log.Printf("Serving request: %s %s from %s (User-Agent: %s) via %s endpoint", r.Method, r.URL.Path, ip, r.UserAgent(), endpoint)
		host, _ := os.Hostname()

		// Get the current time in human-readable format with milliseconds
		timestamp := time.Now().Format("2006-01-02T15:04:05.999Z07:00")

		// Create the response struct with the timestamp as the first field
		response := Response{
			Timestamp: timestamp,
			Message:   messagePtr,
			Hostname:  host,
			Endpoint:  endpoint,
			Node:      nodePtr,
			SourceIP:  ip,
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
			log.Printf("Error encoding JSON response: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}
}

// handleTCPConnection handles a TCP connection and sends the JSON response.
func handleTCPConnection(conn net.Conn, messagePtr *string, nodePtr *string, endpoint string) {
	defer conn.Close()

	// Get the IP address without the port number
	ip, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		log.Printf("Error getting remote address: %v", err)
		return
	}

	// Log the serving request with detailed information
	log.Printf("Serving TCP request from %s via %s endpoint", ip, endpoint)
	host, _ := os.Hostname()

	// Get the current time in human-readable format with milliseconds
	timestamp := time.Now().Format("2006-01-02T15:04:05.999Z07:00")

	// Create the response struct with the timestamp as the first field
	response := Response{
		Timestamp: timestamp,
		Message:   messagePtr,
		Hostname:  host,
		Endpoint:  endpoint,
		Node:      nodePtr,
		SourceIP:  ip,
	}

	// Encode the response struct to JSON and send it as the response
	err = json.NewEncoder(conn).Encode(response)
	if err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

// getMessagePtr gets the MESSAGE environment variable and returns a pointer to it, or nil if it's not set.
func getMessagePtr() *string {
	message := os.Getenv("MESSAGE")
	if message == "" {
		return nil
	}
	return &message
}

// getNodePtr gets the NODE environment variable and returns a pointer to it, or nil if it's not set.
func getNodePtr() *string {
	node := os.Getenv("NODE")
	if node == "" {
		return nil
	}
	return &node
}

// getPrintHeadersSetting checks the PRINT_HTTP_REQUEST_HEADERS environment variable.
func getPrintHeadersSetting() bool {
	return strings.ToLower(os.Getenv("PRINT_HTTP_REQUEST_HEADERS")) == "true"
}

// getTLSSetting checks the TLS environment variable.
func getTLSSetting() bool {
	return strings.ToLower(os.Getenv("TLS")) == "true"
}

// getTCPSetting checks the TCP environment variable.
func getTCPSetting() bool {
	return strings.ToLower(os.Getenv("TCP")) == "true"
}

// getGRPCSetting checks the GRPC environment variable.
func getGRPCSetting() bool {
	return strings.ToLower(os.Getenv("GRPC")) == "true"
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
