package config

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/utils"
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
	UI                     bool
	PublicURL              string
	ProxyProtocol          bool
	ProxyTrustedCIDRs      []string
	TLSCertFile            string
	TLSKeyFile             string
	TLSClientCAFile        string
	ACMEDomain             string
	ACMECacheDir           string
	ACMEAcceptTOS          bool
	ACMEStaging            bool
	GRPCTLS                bool
	TLS                    bool
	H2C                    bool
	TCP                    bool
	GRPC                   bool
	QUIC                   bool
	Metrics                bool
	HTTPPort               string
	HTTPAdvertisePort      string
	TLSAdvertisePort       string
	TCPAdvertisePort       string
	GRPCAdvertisePort      string
	TLSPort                string
	TCPPort                string
	GRPCPort               string
	QUICPort               string
	QUICHost               string
	QUICAdvertisePort      string
	MetricsPort            string
	LogLevel               logrus.Level
	MaxRequestSize         int64 // Maximum request body size in bytes
	MaxResponseSize        int64 // Maximum echo response size; zero uses the legacy 1 MiB cap
	Protection             protection.Options
	ExternalReadinessProbe ExternalReadinessProbe
}

func Load() (*Config, error) {
	viper.SetEnvPrefix("ECHO_OBSERVATORY")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	// Set default values
	viper.SetDefault("message", "")
	viper.SetDefault("node", "")
	viper.SetDefault("print-http-request-headers", false)
	viper.SetDefault("unsafe-headers", false)
	viper.SetDefault("response-controls", false)
	viper.SetDefault("ui", true)
	viper.SetDefault("public-url", "")
	viper.SetDefault("proxy-protocol", false)
	viper.SetDefault("proxy-trusted-cidrs", "")
	viper.SetDefault("tls-cert-file", "")
	viper.SetDefault("tls-key-file", "")
	viper.SetDefault("tls-client-ca-file", "")
	viper.SetDefault("acme-domain", "")
	viper.SetDefault("acme-cache-dir", "")
	viper.SetDefault("acme-accept-tos", false)
	viper.SetDefault("acme-staging", false)
	viper.SetDefault("grpc-tls", false)
	viper.SetDefault("tls", false)
	viper.SetDefault("h2c", false)
	viper.SetDefault("tcp", false)
	viper.SetDefault("grpc", false)
	viper.SetDefault("quic", false)
	viper.SetDefault("metrics", true)
	viper.SetDefault("http-port", "8080")
	viper.SetDefault("http-advertise-port", "")
	viper.SetDefault("tls-advertise-port", "")
	viper.SetDefault("tcp-advertise-port", "")
	viper.SetDefault("grpc-advertise-port", "")
	viper.SetDefault("tls-port", "8443")
	viper.SetDefault("tcp-port", "9090")
	viper.SetDefault("grpc-port", "50051")
	viper.SetDefault("quic-port", "4433")
	viper.SetDefault("quic-host", "")
	viper.SetDefault("quic-advertise-port", "")
	viper.SetDefault("metrics-port", "3000")
	viper.SetDefault("log-level", "info")
	viper.SetDefault("max-request-size", 65536)
	viper.SetDefault("max-response-size", 65536)
	viper.SetDefault("protection", true)
	viper.SetDefault("request-rate", 20)
	viper.SetDefault("peer-request-rate", 5)
	viper.SetDefault("ingress-bytes-per-second", 262144)
	viper.SetDefault("egress-bytes-per-second", 1048576)
	viper.SetDefault("daily-egress-bytes", 1073741824)
	viper.SetDefault("transfer-state-file", "")
	viper.SetDefault("external-readiness-probe-type", "none")
	viper.SetDefault("external-readiness-probe-target", "")
	viper.SetDefault("external-readiness-probe-interval", "10s")
	viper.SetDefault("external-readiness-probe-timeout", "2s")
	viper.SetDefault("external-readiness-http-method", "GET")
	viper.SetDefault("external-readiness-http-expected-status", 200)

	// Load configuration from viper
	cfg := &Config{
		Message:           viper.GetString("message"),
		Node:              viper.GetString("node"),
		PrintHeaders:      viper.GetBool("print-http-request-headers"),
		UnsafeHeaders:     viper.GetBool("unsafe-headers"),
		ResponseControls:  viper.GetBool("response-controls"),
		UI:                viper.GetBool("ui"),
		PublicURL:         viper.GetString("public-url"),
		ProxyProtocol:     viper.GetBool("proxy-protocol"),
		TLSCertFile:       viper.GetString("tls-cert-file"),
		TLSKeyFile:        viper.GetString("tls-key-file"),
		TLSClientCAFile:   viper.GetString("tls-client-ca-file"),
		ACMEDomain:        viper.GetString("acme-domain"),
		ACMECacheDir:      viper.GetString("acme-cache-dir"),
		ACMEAcceptTOS:     viper.GetBool("acme-accept-tos"),
		ACMEStaging:       viper.GetBool("acme-staging"),
		GRPCTLS:           viper.GetBool("grpc-tls"),
		TLS:               viper.GetBool("tls"),
		H2C:               viper.GetBool("h2c"),
		TCP:               viper.GetBool("tcp"),
		GRPC:              viper.GetBool("grpc"),
		QUIC:              viper.GetBool("quic"),
		Metrics:           viper.GetBool("metrics"),
		HTTPPort:          viper.GetString("http-port"),
		HTTPAdvertisePort: viper.GetString("http-advertise-port"),
		TLSAdvertisePort:  viper.GetString("tls-advertise-port"),
		TCPAdvertisePort:  viper.GetString("tcp-advertise-port"),
		GRPCAdvertisePort: viper.GetString("grpc-advertise-port"),
		TLSPort:           viper.GetString("tls-port"),
		TCPPort:           viper.GetString("tcp-port"),
		GRPCPort:          viper.GetString("grpc-port"),
		QUICPort:          viper.GetString("quic-port"),
		QUICHost:          viper.GetString("quic-host"),
		QUICAdvertisePort: viper.GetString("quic-advertise-port"),
		MetricsPort:       viper.GetString("metrics-port"),
		MaxRequestSize:    viper.GetInt64("max-request-size"),
		MaxResponseSize:   viper.GetInt64("max-response-size"),
		Protection: protection.Options{
			Enabled:     viper.GetBool("protection"),
			RequestRate: viper.GetFloat64("request-rate"),
			PeerRate:    viper.GetFloat64("peer-request-rate"),
			IngressRate: viper.GetInt64("ingress-bytes-per-second"),
			EgressRate:  viper.GetInt64("egress-bytes-per-second"),
			DailyEgress: viper.GetInt64("daily-egress-bytes"),
			StateFile:   viper.GetString("transfer-state-file"),
		},
		ExternalReadinessProbe: ExternalReadinessProbe{
			Type:               strings.ToLower(viper.GetString("external-readiness-probe-type")),
			Target:             viper.GetString("external-readiness-probe-target"),
			Interval:           viper.GetDuration("external-readiness-probe-interval"),
			Timeout:            viper.GetDuration("external-readiness-probe-timeout"),
			HTTPMethod:         strings.ToUpper(viper.GetString("external-readiness-http-method")),
			HTTPExpectedStatus: viper.GetInt("external-readiness-http-expected-status"),
		},
	}
	if trustedCIDRs := viper.GetString("proxy-trusted-cidrs"); trustedCIDRs != "" {
		cfg.ProxyTrustedCIDRs = strings.Split(trustedCIDRs, ",")
		for i := range cfg.ProxyTrustedCIDRs {
			cfg.ProxyTrustedCIDRs[i] = strings.TrimSpace(cfg.ProxyTrustedCIDRs[i])
		}
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
	if err := protection.ValidateOptions(c.Protection); err != nil {
		return err
	}
	if c.ProxyProtocol && len(c.ProxyTrustedCIDRs) == 0 {
		return fmt.Errorf("PROXY protocol requires explicitly trusted proxy CIDRs")
	}
	if !c.ProxyProtocol && len(c.ProxyTrustedCIDRs) > 0 {
		return fmt.Errorf("PROXY protocol trust CIDRs require proxy-protocol to be enabled")
	}
	for _, cidr := range c.ProxyTrustedCIDRs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil || prefix != prefix.Masked() || prefix.Bits() == 0 || prefix.Addr().IsUnspecified() || prefix.Addr().IsMulticast() || prefix.Addr().Is4In6() {
			return fmt.Errorf("PROXY protocol trust must use canonical, non-unspecified unicast CIDRs without /0")
		}
	}
	if c.PublicURL != "" {
		publicURL, err := normalizePublicURL(c.PublicURL)
		if err != nil {
			return err
		}
		c.PublicURL = publicURL
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return fmt.Errorf("TLS certificate and key files must be supplied together")
	}
	if c.GRPCTLS && !c.GRPC {
		return fmt.Errorf("grpc-tls requires the gRPC listener")
	}
	if (c.TLSCertFile != "" || c.TLSClientCAFile != "") && !c.TLS && !c.QUIC && !c.GRPCTLS {
		return fmt.Errorf("TLS files require a TLS, QUIC or grpc-tls listener")
	}
	if c.ACMEDomain == "" {
		if c.ACMECacheDir != "" || c.ACMEAcceptTOS || c.ACMEStaging {
			return fmt.Errorf("ACME cache, terms acceptance and staging require an ACME domain")
		}
	} else {
		if !validDNSName(c.ACMEDomain) || !strings.Contains(c.ACMEDomain, ".") || c.ACMEDomain != strings.ToLower(c.ACMEDomain) || net.ParseIP(c.ACMEDomain) != nil || strings.HasSuffix(c.ACMEDomain, ".localhost") {
			return fmt.Errorf("ACME domain must be a canonical lowercase ASCII DNS name, without wildcard, IP, scheme, port or path")
		}
		if !c.TLS {
			return fmt.Errorf("ACME requires the HTTPS listener for public TCP port 443 TLS-ALPN validation")
		}
		if strings.TrimSpace(c.ACMECacheDir) == "" || !c.ACMEAcceptTOS {
			return fmt.Errorf("ACME requires a persistent cache directory and explicit terms acceptance")
		}
		if c.TLSCertFile != "" || c.TLSKeyFile != "" || c.TLSClientCAFile != "" {
			return fmt.Errorf("ACME cannot be combined with certificate, key or client CA files")
		}
	}
	if c.QUICHost != "" {
		if !c.QUIC {
			return fmt.Errorf("QUIC host requires the QUIC listener")
		}
		_, ipErr := netip.ParseAddr(c.QUICHost)
		if strings.ContainsAny(c.QUICHost, "/ \t\r\n") || (ipErr != nil && !validDNSName(strings.TrimSuffix(c.QUICHost, "."))) {
			return fmt.Errorf("QUIC host must be a hostname or IP address, without port, path or whitespace")
		}
	}
	for _, advertised := range []struct {
		name, port string
		enabled    bool
	}{
		{"HTTP", c.HTTPAdvertisePort, true}, {"TLS", c.TLSAdvertisePort, c.TLS},
		{"TCP", c.TCPAdvertisePort, c.TCP}, {"gRPC", c.GRPCAdvertisePort, c.GRPC},
	} {
		if advertised.port == "" {
			continue
		}
		port, err := strconv.Atoi(advertised.port)
		if !advertised.enabled || err != nil || !utils.IsValidPort(advertised.port) || strconv.Itoa(port) != advertised.port {
			return fmt.Errorf("%s advertisement requires an enabled listener and a valid port", advertised.name)
		}
	}
	if c.QUICAdvertisePort != "" {
		port, err := strconv.Atoi(c.QUICAdvertisePort)
		if !c.TLS || !c.QUIC || err != nil || !utils.IsValidPort(c.QUICAdvertisePort) || strconv.Itoa(port) != c.QUICAdvertisePort {
			return fmt.Errorf("QUIC advertisement requires HTTPS, QUIC and a valid public UDP port")
		}
	}
	if len(c.Message) > MaxMessageLength {
		return fmt.Errorf("message length (%d) exceeds maximum allowed length (%d)", len(c.Message), MaxMessageLength)
	}
	if c.MaxResponseSize < 0 || c.MaxResponseSize > 1<<20 {
		return fmt.Errorf("maximum response size must be between zero and 1048576 bytes")
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

func normalizePublicURL(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
		strings.ContainsAny(value, "?#\\") || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", fmt.Errorf("public URL must be an absolute HTTP(S) service URL without credentials, query, fragment or whitespace")
	}
	host := u.Hostname()
	ip, ipErr := netip.ParseAddr(host)
	if strings.HasPrefix(u.Host, "[") {
		if ipErr != nil || !ip.Is6() || ip.Zone() != "" {
			return "", fmt.Errorf("public URL must use a valid bracketed IPv6 address")
		}
	} else if strings.Contains(host, ":") || (ipErr != nil && !validDNSName(strings.TrimSuffix(host, "."))) {
		return "", fmt.Errorf("public URL must use a valid hostname or IP address")
	}
	if strings.HasSuffix(u.Host, ":") || (u.Port() != "" && !utils.IsValidPort(u.Port())) {
		return "", fmt.Errorf("public URL must use a valid port")
	}
	prefix := strings.TrimSuffix(u.Path, "/")
	if u.RawPath != "" || strings.Contains(u.Path, "\\") || strings.Contains(u.Path, "//") || strings.IndexFunc(u.Path, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 || (prefix != "" && path.Clean(prefix) != prefix) {
		return "", fmt.Errorf("public URL must use a clean path prefix without encoded segments")
	}
	u.Path = prefix + "/"
	return u.String(), nil
}

func validDNSName(name string) bool {
	if len(name) == 0 || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			valid := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-'
			if !valid {
				return false
			}
		}
	}
	return true
}
