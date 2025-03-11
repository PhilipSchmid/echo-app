package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/metrics"
	"github.com/PhilipSchmid/echo-app/internal/server"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	// Define command-line flags
	pflag.String("message", "", "Custom message")
	pflag.String("node", "", "Node name")
	pflag.Bool("print-http-request-headers", false, "Print HTTP request headers")
	pflag.Bool("tls", false, "Enable TLS server")
	pflag.Bool("tcp", false, "Enable TCP server")
	pflag.Bool("grpc", false, "Enable gRPC server")
	pflag.Bool("quic", false, "Enable QUIC server")
	pflag.Bool("metrics", true, "Enable metrics server")
	pflag.String("http-port", "8080", "HTTP server port")
	pflag.String("tls-port", "8443", "TLS server port")
	pflag.String("tcp-port", "9090", "TCP server port")
	pflag.String("grpc-port", "50051", "gRPC server port")
	pflag.String("quic-port", "4433", "QUIC server port")
	pflag.String("metrics-port", "3000", "Metrics server port")
	pflag.String("log-level", "info", "Log level (debug, info, warn, error)")

	// Parse the flags
	pflag.Parse()

	// Bind the parsed flags to viper
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		logrus.Fatalf("Failed to bind command-line flags: %v", err)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logrus.Fatalf("Failed to load configuration: %v", err)
	}

	// Start servers based on configuration
	go server.StartHTTPServer(cfg)
	if cfg.TLS {
		go server.StartTLSServer(cfg)
	}
	if cfg.TCP {
		go server.StartTCPServer(cfg)
	}
	if cfg.GRPC {
		go server.StartGRPCServer(cfg)
	}
	if cfg.QUIC {
		go server.StartQUICServer(cfg)
	}
	if cfg.Metrics {
		go metrics.StartMetricsServer(cfg)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	logrus.Info("Shutting down servers...")
	// Add graceful shutdown logic here if needed
}
