package server

import (
	"crypto/tls"
	"net/http"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/handlers"
	"github.com/PhilipSchmid/echo-app/internal/utils"
	"github.com/sirupsen/logrus"
)

// StartHTTPServer starts the HTTP server
func StartHTTPServer(cfg *config.Config) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handlers.HTTPHandler(cfg, "HTTP"))
	server := &http.Server{Addr: ":" + cfg.HTTPPort, Handler: mux}

	logrus.Infof("Starting HTTP server on port %s", cfg.HTTPPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logrus.Errorf("HTTP server failed: %v", err)
	}
}

// StartTLSServer starts the HTTPS server with TLS
func StartTLSServer(cfg *config.Config) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handlers.HTTPHandler(cfg, "TLS"))

	// Generate self-signed certificate
	cert, err := utils.GenerateSelfSignedCert()
	if err != nil {
		logrus.Fatalf("Failed to generate self-signed certificate: %v", err)
	}

	// Create TLS server
	server := &http.Server{
		Addr:    ":" + cfg.TLSPort,
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}

	logrus.Infof("Starting HTTPS server on port %s", cfg.TLSPort)
	if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		logrus.Errorf("HTTPS server failed: %v", err)
	}
}
