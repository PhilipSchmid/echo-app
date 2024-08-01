package main

import (
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

func main() {
	// Get MESSAGE, NODE, PRINT_HTTP_REQUEST_HEADERS, TLS, and TCP environment variables
	messagePtr := getMessagePtr()
	nodePtr := getNodePtr()
	printHeaders := getPrintHeadersSetting()
	tlsEnabled := getTLSSetting()
	tcpEnabled := getTCPSetting()

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
