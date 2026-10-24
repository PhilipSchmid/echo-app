package config

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/PhilipSchmid/echo-app/internal/utils"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const (
	// MaxMessageLength is the maximum allowed length for the custom message field
	MaxMessageLength = 1024 // 1 KB
)

type Config struct {
	Message                string
	Node                   string
	PrintHeaders           bool
	UnsafeHeaders          bool
	ResponseControls       bool
	TLSCertFile            string
	TLSKeyFile             string
	TLSClientCAFile        string
	GRPCTLS                bool
	TLS                    bool
	H2C                    bool
	TCP                    bool
	GRPC                   bool
	QUIC                   bool
	Metrics                bool
	HTTPPort               string
	TLSPort                string
	TCPPort                string
	GRPCPort               string
	QUICPort               string
	MetricsPort            string
	LogLevel               logrus.Level
	MaxRequestSize         int64 // Maximum request body size in bytes
	ExternalReadinessProbe ExternalReadinessProbe
}

func Load() (*Config, error) {
	viper.SetEnvPrefix("ECHO_APP")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	// Set default values
	viper.SetDefault("message", "")
	viper.SetDefault("node", "")
	viper.SetDefault("print-http-request-headers", false)
	viper.SetDefault("unsafe-headers", false)
	viper.SetDefault("response-controls", false)
	viper.SetDefault("tls-cert-file", "")
	viper.SetDefault("tls-key-file", "")
	viper.SetDefault("tls-client-ca-file", "")
	viper.SetDefault("grpc-tls", false)
	viper.SetDefault("tls", false)
	viper.SetDefault("h2c", false)
	viper.SetDefault("tcp", false)
	viper.SetDefault("grpc", false)
	viper.SetDefault("quic", false)
	viper.SetDefault("metrics", true)
	viper.SetDefault("http-port", "8080")
	viper.SetDefault("tls-port", "8443")
	viper.SetDefault("tcp-port", "9090")
	viper.SetDefault("grpc-port", "50051")
	viper.SetDefault("quic-port", "4433")
	viper.SetDefault("metrics-port", "3000")
	viper.SetDefault("log-level", "info")
	viper.SetDefault("max-request-size", 10485760) // 10 MB default
	viper.SetDefault("external-readiness-probe-type", "none")
	viper.SetDefault("external-readiness-probe-target", "")
	viper.SetDefault("external-readiness-probe-interval", "10s")
	viper.SetDefault("external-readiness-probe-timeout", "2s")
	viper.SetDefault("external-readiness-http-method", "GET")
	viper.SetDefault("external-readiness-http-expected-status", 200)

	// Load configuration from viper
	cfg := &Config{
		Message:          viper.GetString("message"),
		Node:             viper.GetString("node"),
		PrintHeaders:     viper.GetBool("print-http-request-headers"),
		UnsafeHeaders:    viper.GetBool("unsafe-headers"),
		ResponseControls: viper.GetBool("response-controls"),
		TLSCertFile:      viper.GetString("tls-cert-file"),
		TLSKeyFile:       viper.GetString("tls-key-file"),
		TLSClientCAFile:  viper.GetString("tls-client-ca-file"),
		GRPCTLS:          viper.GetBool("grpc-tls"),
		TLS:              viper.GetBool("tls"),
		H2C:              viper.GetBool("h2c"),
		TCP:              viper.GetBool("tcp"),
		GRPC:             viper.GetBool("grpc"),
		QUIC:             viper.GetBool("quic"),
		Metrics:          viper.GetBool("metrics"),
		HTTPPort:         viper.GetString("http-port"),
		TLSPort:          viper.GetString("tls-port"),
		TCPPort:          viper.GetString("tcp-port"),
		GRPCPort:         viper.GetString("grpc-port"),
		QUICPort:         viper.GetString("quic-port"),
		MetricsPort:      viper.GetString("metrics-port"),
		MaxRequestSize:   viper.GetInt64("max-request-size"),
		ExternalReadinessProbe: ExternalReadinessProbe{
			Type:               strings.ToLower(viper.GetString("external-readiness-probe-type")),
			Target:             viper.GetString("external-readiness-probe-target"),
			Interval:           viper.GetDuration("external-readiness-probe-interval"),
			Timeout:            viper.GetDuration("external-readiness-probe-timeout"),
			HTTPMethod:         strings.ToUpper(viper.GetString("external-readiness-http-method")),
			HTTPExpectedStatus: viper.GetInt("external-readiness-http-expected-status"),
		},
	}

	// Set log level
	lvl, err := logrus.ParseLevel(viper.GetString("log-level"))
	if err != nil {
		return nil, err
	}
	cfg.LogLevel = lvl
	logrus.SetLevel(cfg.LogLevel)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate rejects incomplete settings before any listeners are opened.
func (c *Config) Validate() error {
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return fmt.Errorf("TLS certificate and key files must be supplied together")
	}
	if c.GRPCTLS && !c.GRPC {
		return fmt.Errorf("grpc-tls requires the gRPC listener")
	}
	if (c.TLSCertFile != "" || c.TLSClientCAFile != "") && !c.TLS && !c.QUIC && !c.GRPCTLS {
		return fmt.Errorf("TLS files require a TLS, QUIC or grpc-tls listener")
	}
	if len(c.Message) > MaxMessageLength {
		return fmt.Errorf("message length (%d) exceeds maximum allowed length (%d)", len(c.Message), MaxMessageLength)
	}
	if c.MaxRequestSize <= 0 {
		return fmt.Errorf("maximum request size must be greater than zero")
	}
	ports := []struct {
		name, port string
		enabled    bool
	}{
		{"HTTP", c.HTTPPort, true},
		{"TLS", c.TLSPort, c.TLS},
		{"TCP", c.TCPPort, c.TCP},
		{"gRPC", c.GRPCPort, c.GRPC},
		{"metrics", c.MetricsPort, c.Metrics},
	}
	used := make(map[int]string)
	for _, p := range ports {
		if !p.enabled {
			continue
		}
		if !utils.IsValidPort(p.port) {
			return fmt.Errorf("invalid %s port: %s", p.name, p.port)
		}
		number, _ := strconv.Atoi(p.port)
		if other, exists := used[number]; exists {
			return fmt.Errorf("%s and %s use the same TCP port %s", other, p.name, p.port)
		}
		used[number] = p.name
	}
	if c.QUIC && !utils.IsValidPort(c.QUICPort) {
		return fmt.Errorf("invalid QUIC port: %s", c.QUICPort)
	}
	p := c.ExternalReadinessProbe
	switch p.Type {
	case "", "none":
		if p.Target != "" {
			return fmt.Errorf("external readiness probe target requires a probe type")
		}
		return nil
	case "http", "https", "tcp", "ping", "icmp":
	default:
		return fmt.Errorf("invalid external readiness probe type: %s", p.Type)
	}
	if strings.TrimSpace(p.Target) == "" {
		return fmt.Errorf("external readiness probe target is required")
	}
	if p.Interval <= 0 || p.Timeout <= 0 {
		return fmt.Errorf("external readiness probe interval and timeout must be greater than zero")
	}
	switch p.Type {
	case "http", "https":
		u, err := url.Parse(p.Target)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("external readiness HTTP target must be an HTTP or HTTPS URL")
		}
		if _, err := http.NewRequest(p.HTTPMethod, p.Target, nil); err != nil {
			return fmt.Errorf("invalid external readiness HTTP request: %w", err)
		}
		if p.HTTPExpectedStatus < 100 || p.HTTPExpectedStatus > 599 {
			return fmt.Errorf("external readiness HTTP expected status must be between 100 and 599")
		}
	case "tcp":
		host, port, err := net.SplitHostPort(p.Target)
		if err != nil || host == "" || !utils.IsValidPort(port) {
			return fmt.Errorf("external readiness TCP target must be host:port")
		}
	case "ping", "icmp":
		if strings.ContainsAny(p.Target, "/ \t\r\n") {
			return fmt.Errorf("external readiness ICMP target must be a hostname or IP address")
		}
	}
	return nil
}
