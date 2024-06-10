package main

import (
	"encoding/json"
	"log"
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
	Node      *string             `json:"node,omitempty"`    // Optional field to include node name
	Headers   map[string][]string `json:"headers,omitempty"` // Optional field to include headers
}

func main() {
	// Get MESSAGE and PRINT_HTTP_REQUEST_HEADERS environment variables
	messagePtr := getMessagePtr()
	nodePtr := getNodePtr()
	printHeaders := getPrintHeadersSetting()

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

	// Register hello function to handle all requests
	mux := http.NewServeMux()
	mux.HandleFunc("/", hello(messagePtr, nodePtr, printHeaders)) // Pass message and node pointers and printHeaders to the hello function

	// Use PORT environment variable, or default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start the web server on port and accept requests
	log.Printf("Server listening on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// hello returns a http.HandlerFunc that uses the provided message pointer and printHeaders flag.
func hello(messagePtr *string, nodePtr *string, printHeaders bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get the IP address without the port number
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			log.Printf("Error getting remote address: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Log the serving request with the source IP
		log.Printf("Serving request: %s from %s", r.URL.Path, ip)
		host, _ := os.Hostname()

		// Get the current time in human-readable format with milliseconds
		timestamp := time.Now().Format("2006-01-02T15:04:05.999Z07:00")

		// Create the response struct with the timestamp as the first field
		response := Response{
			Timestamp: timestamp,
			Message:   messagePtr,
			Hostname:  host,
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
