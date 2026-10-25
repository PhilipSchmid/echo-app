package main

import (
	"context"
	"crypto/tls"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/health"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/server"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func main() {
	pflag.Bool("response-controls", false, "Enable bounded status, delay and size query controls")
	pflag.Bool("ui", true, "Enable the browser interface at /ui/ on HTTP, HTTPS and HTTP/3")
	pflag.String("public-url", "", "Public service base URL for UI search discovery; empty disables indexing")
	// Define command-line flags
	pflag.String("message", "", "Custom message")
	pflag.String("node", "", "Node name")
	pflag.Bool("print-http-request-headers", false, "Print HTTP request headers")
	pflag.Bool("unsafe-headers", false, "Include sensitive values in diagnostic headers")
	pflag.String("tls-cert-file", "", "PEM server certificate for HTTPS, QUIC and TLS-enabled gRPC")
	pflag.String("tls-key-file", "", "PEM private key for the server certificate")
	pflag.String("tls-client-ca-file", "", "PEM client CA bundle; require and verify client certificates")
	pflag.String("acme-domain", "", "Obtain and renew a Let's Encrypt certificate for this DNS name (requires HTTPS on public port 443)")
	pflag.String("acme-cache-dir", "", "Persistent private directory for ACME account keys and certificates")
	pflag.Bool("acme-accept-tos", false, "Accept the Let's Encrypt subscriber agreement for opt-in ACME")
	pflag.Bool("acme-staging", false, "Use Let's Encrypt staging with untrusted test certificates")
	pflag.Bool("grpc-tls", false, "Enable TLS on the gRPC listener")
	pflag.Bool("tls", false, "Enable TLS server")
	pflag.Bool("h2c", false, "Enable HTTP/2 cleartext (h2c) on the HTTP listener")
	pflag.Bool("tcp", false, "Enable TCP server")
	pflag.Bool("grpc", false, "Enable gRPC server")
	pflag.Bool("quic", false, "Enable QUIC server")
	pflag.Bool("metrics", true, "Enable metrics server")
	pflag.String("http-port", "8080", "HTTP server port")
	pflag.String("http-advertise-port", "", "Reachable HTTP port shown in connection examples; empty uses the bind port")
	pflag.String("tls-advertise-port", "", "Reachable HTTPS port shown in connection examples; empty uses the bind port")
	pflag.String("tcp-advertise-port", "", "Reachable raw TCP port shown in connection examples; empty uses the bind port")
	pflag.String("grpc-advertise-port", "", "Reachable gRPC port shown in connection examples; empty uses the bind port")
	pflag.String("tls-port", "8443", "TLS server port")
	pflag.String("tcp-port", "9090", "TCP server port")
	pflag.String("grpc-port", "50051", "gRPC server port")
	pflag.String("quic-port", "4433", "QUIC server port")
	pflag.String("quic-host", "", "QUIC bind hostname or IP; empty binds all interfaces")
	pflag.String("quic-advertise-port", "", "Advertise this reachable public UDP port via HTTPS Alt-Svc; empty disables discovery")
	pflag.String("metrics-port", "3000", "Metrics server port")
	pflag.String("log-level", "info", "Log level (debug, info, warn, error)")
	pflag.Int64("max-request-size", 65536, "Maximum request body or message size in bytes")
	pflag.Int64("max-response-size", 65536, "Maximum echo response size in bytes (0 uses the legacy 1 MiB cap)")
	pflag.Bool("protection", true, "Enable shared public listener rate and transfer budgets")
	pflag.Float64("request-rate", 20, "Global operations and new connections per second (independent buckets)")
	pflag.Float64("peer-request-rate", 5, "Operations and new connections per second per socket peer (0 disables)")
	pflag.Int64("ingress-bytes-per-second", 262144, "Shared public ingress byte rate")
	pflag.Int64("egress-bytes-per-second", 1048576, "Shared public egress byte rate")
	pflag.Int64("daily-egress-bytes", 1073741824, "Public outbound byte allowance per UTC day")
	pflag.String("transfer-state-file", "", "Private file preserving the daily budget across restarts; empty uses memory")
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
	var sharedTLS *tls.Config
	var certificateSource handlers.CertificateSource
	if cfg.TLS || cfg.GRPCTLS || cfg.QUIC {
		sharedTLS, err = handlers.GetTLSConfig(cfg)
		if err != nil {
			logrus.Fatalf("Failed to configure TLS: %v", err)
		}
		certificateSource = handlers.TrackServerCertificate(sharedTLS)
	}

	guard, err := protection.NewGuard(cfg.Protection)
	if err != nil {
		logrus.Fatalf("Failed to configure protection: %v", err)
	}
	defer func() { _ = guard.Close() }()
	metrics.RegisterProtection(guard)

	// Create shared health/readiness checker
	healthChecker := health.NewChecker(cfg.ExternalReadinessProbe)

	// Create server manager
	manager := server.NewManager(cfg, healthChecker)

	// Register servers based on configuration
	// Always start HTTP server
	httpServer := server.NewHTTPServer(cfg, false, nil, certificateSource)
	httpServer.SetProtection(guard)
	manager.RegisterServer(httpServer)

	if cfg.TLS {
		tlsServer := server.NewHTTPServer(cfg, true, sharedTLS, certificateSource)
		tlsServer.SetProtection(guard)
		manager.RegisterServer(tlsServer)
	}
	if cfg.TCP {
		tcpServer := server.NewTCPServer(cfg)
		tcpServer.SetProtection(guard)
		manager.RegisterServer(tcpServer)
	}
	if cfg.GRPC {
		grpcServer := server.NewGRPCServer(cfg, healthChecker, sharedTLS)
		grpcServer.SetProtection(guard)
		manager.RegisterServer(grpcServer)
	}
	if cfg.QUIC {
		quicServer := server.NewQUICServer(cfg, sharedTLS, certificateSource)
		quicServer.SetProtection(guard)
		manager.RegisterServer(quicServer)
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
	if cfg.ACMEDomain != "" {
		// TLS-ALPN validation needs the HTTPS listener running before issuance starts.
		go warmCertificate(ctx, cfg.ACMEDomain, sharedTLS.GetCertificate, time.Minute)
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

func warmCertificate(ctx context.Context, domain string, getCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error), retry time.Duration) {
	hello := &tls.ClientHelloInfo{
		ServerName: domain, SupportedProtos: []string{"h2", "http/1.1"},
		CipherSuites:     []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
		SignatureSchemes: []tls.SignatureScheme{tls.ECDSAWithP256AndSHA256},
		SupportedCurves:  []tls.CurveID{tls.CurveP256}, SupportedVersions: []uint16{tls.VersionTLS13, tls.VersionTLS12},
	}
	for ctx.Err() == nil {
		if _, err := getCertificate(hello); err == nil {
			logrus.Infof("ACME certificate ready for %s; automatic renewal enabled", domain)
			return
		} else {
			logrus.Warnf("ACME certificate for %s is not ready; retrying in %s: %v", domain, retry, err)
		}
		timer := time.NewTimer(retry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
