package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/health"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/server"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	pflag.Bool("response-controls", false, "Enable bounded status, delay and size query controls")
	// Define command-line flags
	pflag.String("message", "", "Custom message")
	pflag.String("node", "", "Node name")
	pflag.Bool("print-http-request-headers", false, "Print HTTP request headers")
	pflag.Bool("unsafe-headers", false, "Include sensitive values in diagnostic headers")
	pflag.String("tls-cert-file", "", "PEM server certificate for HTTPS, QUIC and TLS-enabled gRPC")
	pflag.String("tls-key-file", "", "PEM private key for the server certificate")
	pflag.String("tls-client-ca-file", "", "PEM client CA bundle; require and verify client certificates")
	pflag.Bool("grpc-tls", false, "Enable TLS on the gRPC listener")
	pflag.Bool("tls", false, "Enable TLS server")
	pflag.Bool("h2c", false, "Enable HTTP/2 cleartext (h2c) on the HTTP listener")
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
	pflag.Int64("max-request-size", 10485760, "Maximum request body size in bytes (default: 10MB)")
	pflag.String("external-readiness-probe-type", "none", "External readiness probe type: none, http, tcp, or icmp")
	pflag.String("external-readiness-probe-target", "", "External readiness probe target URL, host:port, or host/IP")
	pflag.Duration("external-readiness-probe-interval", 10*time.Second, "External readiness probe interval")
	pflag.Duration("external-readiness-probe-timeout", 2*time.Second, "External readiness probe timeout")
	pflag.String("external-readiness-http-method", "GET", "HTTP method for external readiness HTTP probes")
	pflag.Int("external-readiness-http-expected-status", 200, "Expected HTTP status for external readiness HTTP probes")

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

	// Create shared health/readiness checker
	healthChecker := health.NewChecker(cfg.ExternalReadinessProbe)

	// Create server manager
	manager := server.NewManager(cfg, healthChecker)

	// Register servers based on configuration
	// Always start HTTP server
	manager.RegisterServer(server.NewHTTPServer(cfg, false))

	if cfg.TLS {
		manager.RegisterServer(server.NewHTTPServer(cfg, true))
	}
	if cfg.TCP {
		manager.RegisterServer(server.NewTCPServer(cfg))
	}
	if cfg.GRPC {
		manager.RegisterServer(server.NewGRPCServer(cfg, healthChecker))
	}
	if cfg.QUIC {
		manager.RegisterServer(server.NewQUICServer(cfg))
	}
	if cfg.Metrics {
		manager.RegisterServer(server.NewMetricsServer(cfg, healthChecker))
	}

	// Create context for server lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Start all servers
	healthChecker.Start(ctx)

	if err := manager.Start(ctx); err != nil {
		logrus.Errorf("Failed to start servers: %v", err)
		_ = manager.Shutdown(30 * time.Second)
		os.Exit(1)
	}

	// A listener failure is fatal even when other listeners remain available.
	failed := false
	select {
	case <-sigChan:
	case err := <-manager.Errors():
		logrus.Errorf("Listener failed: %v", err)
		failed = true
	}

	// Graceful shutdown with timeout
	shutdownTimeout := 30 * time.Second
	logrus.Infof("Shutting down servers (timeout: %v)...", shutdownTimeout)

	if err := manager.Shutdown(shutdownTimeout); err != nil {
		logrus.Errorf("Shutdown error: %v", err)
		os.Exit(1)
	}

	cancel()
	if failed {
		os.Exit(1)
	}
	logrus.Info("Shutdown complete")
}
