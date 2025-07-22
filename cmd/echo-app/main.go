package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/server"
	"github.com/PhilipSchmid/echo-app/internal/utils"
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

	// Validate configuration
	if err := validateConfig(cfg); err != nil {
		logrus.Fatalf("Invalid configuration: %v", err)
	}

	// Create server manager
	manager := server.NewManager(cfg)

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
		manager.RegisterServer(server.NewGRPCServer(cfg))
	}
	if cfg.QUIC {
		manager.RegisterServer(server.NewQUICServer(cfg))
	}
	if cfg.Metrics {
		manager.RegisterServer(server.NewMetricsServer(cfg))
	}

	// Create context for server lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start all servers
	if err := manager.Start(ctx); err != nil {
		logrus.Errorf("Failed to start servers: %v", err)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Cancel context to signal shutdown
	cancel()

	// Graceful shutdown with timeout
	shutdownTimeout := 30 * time.Second
	logrus.Infof("Shutting down servers (timeout: %v)...", shutdownTimeout)

	if err := manager.Shutdown(shutdownTimeout); err != nil {
		logrus.Errorf("Shutdown error: %v", err)
		os.Exit(1)
	}

	logrus.Info("Shutdown complete")
}

// validateConfig validates the configuration
func validateConfig(cfg *config.Config) error {
	// Validate ports
	if !utils.IsValidPort(cfg.HTTPPort) {
		return fmt.Errorf("invalid HTTP port: %s", cfg.HTTPPort)
	}
	if cfg.TLS && !utils.IsValidPort(cfg.TLSPort) {
		return fmt.Errorf("invalid TLS port: %s", cfg.TLSPort)
	}
	if cfg.TCP && !utils.IsValidPort(cfg.TCPPort) {
		return fmt.Errorf("invalid TCP port: %s", cfg.TCPPort)
	}
	if cfg.GRPC && !utils.IsValidPort(cfg.GRPCPort) {
		return fmt.Errorf("invalid gRPC port: %s", cfg.GRPCPort)
	}
	if cfg.QUIC && !utils.IsValidPort(cfg.QUICPort) {
		return fmt.Errorf("invalid QUIC port: %s", cfg.QUICPort)
	}
	if cfg.Metrics && !utils.IsValidPort(cfg.MetricsPort) {
		return fmt.Errorf("invalid metrics port: %s", cfg.MetricsPort)
	}

	return nil
}
