package main

import (
    "encoding/json"
    "log"
    "net"
    "net/http"
    "os"
)

// Response is the struct for the JSON response
type Response struct {
    Message   string `json:"message"`
    Hostname  string `json:"hostname"`
    SourceIP  string `json:"source_ip"`
}

func main() {
    // Use MESSAGE environment variable, or default to "Hello, world!"
    message := os.Getenv("MESSAGE")
    if message == "" {
        message = "Hello, world!"
    }

    // Register hello function to handle all requests
    mux := http.NewServeMux()
    mux.HandleFunc("/", hello(message)) // Pass message to the hello function

    // Use PORT environment variable, or default to 8080
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    // Start the web server on port and accept requests
    log.Printf("Server listening on port %s", port)
    log.Fatal(http.ListenAndServe(":"+port, mux))
}

// hello returns a http.HandlerFunc that uses the provided message.
func hello(message string) http.HandlerFunc {
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

        // Create the response struct
        response := Response{
            Message:   message,
            Hostname:  host,
            SourceIP:  ip,
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