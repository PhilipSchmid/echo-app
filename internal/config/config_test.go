package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/protection"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultValues(t *testing.T) {
	// Reset viper before each test
	viper.Reset()

	cfg, err := Load()
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	// Verify default values
	assert.Equal(t, "", cfg.Message)
	assert.Equal(t, "", cfg.Node)
	assert.False(t, cfg.PrintHeaders)
	assert.False(t, cfg.ResponseControls)
	assert.True(t, cfg.UI)
	assert.Empty(t, cfg.PublicURL)
	assert.False(t, cfg.ProxyProtocol)
	assert.Empty(t, cfg.ProxyTrustedCIDRs)
	assert.False(t, cfg.GRPCTLS)
	assert.Empty(t, cfg.TLSCertFile)
	assert.Empty(t, cfg.TLSKeyFile)
	assert.Empty(t, cfg.TLSClientCAFile)
	assert.Empty(t, cfg.ACMEDomain)
	assert.Empty(t, cfg.ACMECacheDir)
	assert.False(t, cfg.ACMEAcceptTOS)
	assert.False(t, cfg.ACMEStaging)
	assert.False(t, cfg.TLS)
	assert.False(t, cfg.TCP)
	assert.False(t, cfg.GRPC)
	assert.False(t, cfg.QUIC)
	assert.False(t, cfg.QUICDisablePMTUD)
	assert.True(t, cfg.Metrics)
	assert.Equal(t, "8080", cfg.HTTPPort)
	assert.Equal(t, "8443", cfg.TLSPort)
	assert.Equal(t, "9090", cfg.TCPPort)
	assert.Equal(t, "50051", cfg.GRPCPort)
	assert.Equal(t, "4433", cfg.QUICPort)
	assert.Empty(t, cfg.QUICHost)
	assert.Equal(t, "3000", cfg.MetricsPort)
	assert.Equal(t, int64(65536), cfg.MaxRequestSize)
	assert.Equal(t, int64(65536), cfg.MaxResponseSize)
	assert.Equal(t, protection.Options{
		Enabled: true, RequestRate: 20, PeerRate: 5,
		IngressRate: 262144, EgressRate: 1048576, DailyEgress: 1073741824,
	}, cfg.Protection)
	assert.Equal(t, logrus.InfoLevel, cfg.LogLevel)
}

func TestQUICPathMTUEnvironmentAndFlagPrecedence(t *testing.T) {
	for _, test := range []struct {
		name, environment string
		flags             []string
		want              bool
	}{
		{"environment", "true", nil, true},
		{"flag enables", "false", []string{"--quic-disable-path-mtu-discovery"}, true},
		{"flag disables", "true", []string{"--quic-disable-path-mtu-discovery=false"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			t.Setenv("ECHO_OBSERVATORY_QUIC_DISABLE_PATH_MTU_DISCOVERY", test.environment)
			flags := pflag.NewFlagSet("quic", pflag.ContinueOnError)
			flags.Bool("quic-disable-path-mtu-discovery", false, "")
			require.NoError(t, flags.Parse(test.flags))
			require.NoError(t, viper.BindPFlags(flags))
			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, test.want, cfg.QUICDisablePMTUD)
		})
	}
}

func TestLoad_Protection(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ECHO_OBSERVATORY_REQUEST_RATE", "2.5")
	t.Setenv("ECHO_OBSERVATORY_PEER_REQUEST_RATE", "0")
	t.Setenv("ECHO_OBSERVATORY_INGRESS_BYTES_PER_SECOND", "1024")
	t.Setenv("ECHO_OBSERVATORY_EGRESS_BYTES_PER_SECOND", "2048")
	t.Setenv("ECHO_OBSERVATORY_DAILY_EGRESS_BYTES", "4096")
	t.Setenv("ECHO_OBSERVATORY_TRANSFER_STATE_FILE", "/not-opened-during-validation/budget.json")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, protection.Options{
		Enabled: true, RequestRate: 2.5, PeerRate: 0,
		IngressRate: 1024, EgressRate: 2048, DailyEgress: 4096,
		StateFile: "/not-opened-during-validation/budget.json",
	}, cfg.Protection)

	t.Setenv("ECHO_OBSERVATORY_REQUEST_RATE", "0")
	_, err = Load()
	require.ErrorContains(t, err, "request rate")
	t.Setenv("ECHO_OBSERVATORY_PROTECTION", "false")
	cfg, err = Load()
	require.NoError(t, err)
	assert.False(t, cfg.Protection.Enabled)
}

func TestProxyProtocolEnvironmentAndFlagPrecedence(t *testing.T) {
	for _, test := range []struct {
		name, environment, cidrs string
		flags                    []string
		want                     []string
	}{
		{"environment", "true", "192.0.2.10/32, 2001:db8:1::10/128", nil, []string{"192.0.2.10/32", "2001:db8:1::10/128"}},
		{"flag enables environment", "false", "", []string{"--proxy-protocol", "--proxy-trusted-cidrs=127.0.0.1/32,::1/128"}, []string{"127.0.0.1/32", "::1/128"}},
		{"flag replaces environment list", "true", "192.0.2.10/32", []string{"--proxy-trusted-cidrs=10.20.30.0/24"}, []string{"10.20.30.0/24"}},
		{"flag disables and clears environment", "true", "192.0.2.10/32", []string{"--proxy-protocol=false", "--proxy-trusted-cidrs="}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			t.Setenv("ECHO_OBSERVATORY_PROXY_PROTOCOL", test.environment)
			t.Setenv("ECHO_OBSERVATORY_PROXY_TRUSTED_CIDRS", test.cidrs)
			flags := pflag.NewFlagSet("proxy", pflag.ContinueOnError)
			flags.Bool("proxy-protocol", false, "")
			flags.String("proxy-trusted-cidrs", "", "")
			require.NoError(t, flags.Parse(test.flags))
			require.NoError(t, viper.BindPFlags(flags))
			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, len(test.want) > 0, cfg.ProxyProtocol)
			assert.Equal(t, test.want, cfg.ProxyTrustedCIDRs)
		})
	}
}

func TestValidateProxyProtocolTrust(t *testing.T) {
	for _, test := range []struct {
		name    string
		enabled bool
		cidrs   []string
	}{
		{"missing trust", true, nil},
		{"unused trust", false, []string{"192.0.2.10/32"}},
		{"empty entry", true, []string{""}},
		{"missing prefix", true, []string{"192.0.2.10"}},
		{"invalid prefix", true, []string{"192.0.2.10/33"}},
		{"IPv4 host bits", true, []string{"192.0.2.10/24"}},
		{"IPv6 host bits", true, []string{"2001:db8::1/64"}},
		{"IPv4 all peers", true, []string{"0.0.0.0/0"}},
		{"IPv6 all peers", true, []string{"::/0"}},
		{"unspecified peer", true, []string{"0.0.0.0/32"}},
		{"IPv6 unspecified peer", true, []string{"::/128"}},
		{"mapped IPv4", true, []string{"::ffff:192.0.2.10/128"}},
		{"multicast IPv4", true, []string{"224.0.0.0/4"}},
		{"multicast IPv6", true, []string{"ff00::/8"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := &Config{HTTPPort: "8080", MaxRequestSize: 1024, ProxyProtocol: test.enabled, ProxyTrustedCIDRs: test.cidrs}
			require.ErrorContains(t, cfg.Validate(), "PROXY protocol")
		})
	}
	for _, cidr := range []string{"127.0.0.1/32", "::1/128", "192.0.2.10/32", "10.20.30.0/24", "fd00::/8", "2001:db8:1::10/128"} {
		t.Run("valid/"+cidr, func(t *testing.T) {
			cfg := &Config{HTTPPort: "8080", MaxRequestSize: 1024, ProxyProtocol: true, ProxyTrustedCIDRs: []string{cidr}}
			require.NoError(t, cfg.Validate())
		})
	}
}

func TestLoadRejectsEmptyProxyTrustEntries(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ECHO_OBSERVATORY_PROXY_PROTOCOL", "true")
	t.Setenv("ECHO_OBSERVATORY_PROXY_TRUSTED_CIDRS", "192.0.2.10/32,,::1/128")
	_, err := Load()
	require.ErrorContains(t, err, "PROXY protocol")
}

func TestLoad_EnvironmentVariables(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(t *testing.T, cfg *Config)
	}{
		{
			name:     "response controls",
			envVars:  map[string]string{"ECHO_OBSERVATORY_RESPONSE_CONTROLS": "true"},
			validate: func(t *testing.T, cfg *Config) { assert.True(t, cfg.ResponseControls) },
		},
		{
			name:    "TLS files and gRPC TLS",
			envVars: map[string]string{"ECHO_OBSERVATORY_GRPC": "true", "ECHO_OBSERVATORY_GRPC_TLS": "true", "ECHO_OBSERVATORY_TLS_CERT_FILE": "server.pem", "ECHO_OBSERVATORY_TLS_KEY_FILE": "server.key", "ECHO_OBSERVATORY_TLS_CLIENT_CA_FILE": "ca.pem"},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.GRPCTLS)
				assert.Equal(t, "server.pem", cfg.TLSCertFile)
				assert.Equal(t, "server.key", cfg.TLSKeyFile)
				assert.Equal(t, "ca.pem", cfg.TLSClientCAFile)
			},
		},
		{
			name: "custom message",
			envVars: map[string]string{
				"ECHO_OBSERVATORY_MESSAGE": "test-message",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "test-message", cfg.Message)
			},
		},
		{
			name: "custom node",
			envVars: map[string]string{
				"ECHO_OBSERVATORY_NODE": "test-node",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "test-node", cfg.Node)
			},
		},
		{
			name: "enable TLS",
			envVars: map[string]string{
				"ECHO_OBSERVATORY_TLS": "true",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.TLS)
			},
		},
		{
			name: "enable TCP",
			envVars: map[string]string{
				"ECHO_OBSERVATORY_TCP": "true",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.TCP)
			},
		},
		{
			name: "enable GRPC",
			envVars: map[string]string{
				"ECHO_OBSERVATORY_GRPC": "true",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.GRPC)
			},
		},
		{
			name: "enable QUIC",
			envVars: map[string]string{
				"ECHO_OBSERVATORY_QUIC": "true",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.QUIC)
			},
		},
		{
			name: "print headers",
			envVars: map[string]string{
				"ECHO_OBSERVATORY_PRINT_HTTP_REQUEST_HEADERS": "true",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.PrintHeaders)
			},
		},
		{
			name: "custom ports",
			envVars: map[string]string{
				"ECHO_OBSERVATORY_HTTP_PORT":    "9000",
				"ECHO_OBSERVATORY_TLS_PORT":     "9443",
				"ECHO_OBSERVATORY_TCP_PORT":     "9999",
				"ECHO_OBSERVATORY_GRPC_PORT":    "50052",
				"ECHO_OBSERVATORY_QUIC_PORT":    "4444",
				"ECHO_OBSERVATORY_METRICS_PORT": "3001",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "9000", cfg.HTTPPort)
				assert.Equal(t, "9443", cfg.TLSPort)
				assert.Equal(t, "9999", cfg.TCPPort)
				assert.Equal(t, "50052", cfg.GRPCPort)
				assert.Equal(t, "4444", cfg.QUICPort)
				assert.Equal(t, "3001", cfg.MetricsPort)
			},
		},
		{
			name: "custom max request size",
			envVars: map[string]string{
				"ECHO_OBSERVATORY_MAX_REQUEST_SIZE": "5242880", // 5MB
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, int64(5242880), cfg.MaxRequestSize)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper before each test
			viper.Reset()

			// Set environment variables
			for key, value := range tt.envVars {
				_ = os.Setenv(key, value)
				defer func(k string) { _ = os.Unsetenv(k) }(key)
			}

			cfg, err := Load()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			tt.validate(t, cfg)
		})
	}
}

func TestLoad_MessageLengthValidation(t *testing.T) {
	// Helper to create a string of specific length
	createMessage := func(length int) string {
		result := ""
		for i := 0; i < length; i++ {
			result += "a"
		}
		return result
	}

	tests := []struct {
		name        string
		message     string
		expectError bool
	}{
		{
			name:        "empty message",
			message:     "",
			expectError: false,
		},
		{
			name:        "short message",
			message:     "test",
			expectError: false,
		},
		{
			name:        "message at limit (1024 bytes)",
			message:     createMessage(1024),
			expectError: false,
		},
		{
			name:        "message over limit (1025 bytes)",
			message:     createMessage(1025),
			expectError: true,
		},
		{
			name:        "message way over limit (2048 bytes)",
			message:     createMessage(2048),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper before each test
			viper.Reset()

			_ = os.Setenv("ECHO_OBSERVATORY_MESSAGE", tt.message)
			defer func() { _ = os.Unsetenv("ECHO_OBSERVATORY_MESSAGE") }()

			cfg, err := Load()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, cfg)
				assert.Contains(t, err.Error(), "message length")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)
				assert.Equal(t, tt.message, cfg.Message)
			}
		})
	}
}

func TestLoad_LogLevelParsing(t *testing.T) {
	tests := []struct {
		name        string
		logLevel    string
		expected    logrus.Level
		expectError bool
	}{
		{
			name:        "debug level",
			logLevel:    "debug",
			expected:    logrus.DebugLevel,
			expectError: false,
		},
		{
			name:        "info level",
			logLevel:    "info",
			expected:    logrus.InfoLevel,
			expectError: false,
		},
		{
			name:        "warn level",
			logLevel:    "warn",
			expected:    logrus.WarnLevel,
			expectError: false,
		},
		{
			name:        "error level",
			logLevel:    "error",
			expected:    logrus.ErrorLevel,
			expectError: false,
		},
		{
			name:        "invalid level",
			logLevel:    "invalid",
			expected:    logrus.InfoLevel,
			expectError: true,
		},
		{
			name:        "empty level defaults to info",
			logLevel:    "",
			expected:    logrus.InfoLevel,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper before each test
			viper.Reset()

			if tt.logLevel != "" {
				_ = os.Setenv("ECHO_OBSERVATORY_LOG_LEVEL", tt.logLevel)
				defer func() { _ = os.Unsetenv("ECHO_OBSERVATORY_LOG_LEVEL") }()
			}

			cfg, err := Load()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, cfg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)
				assert.Equal(t, tt.expected, cfg.LogLevel)
				// Verify logrus global level is set
				assert.Equal(t, tt.expected, logrus.GetLevel())
			}
		})
	}
}

func TestLoad_MaxMessageLengthConstant(t *testing.T) {
	// Verify the constant value is as expected
	assert.Equal(t, 1024, MaxMessageLength)
}

func TestLoad_CombinedConfiguration(t *testing.T) {
	// Reset viper before test
	viper.Reset()

	// Set multiple environment variables
	_ = os.Setenv("ECHO_OBSERVATORY_MESSAGE", "prod-env")
	_ = os.Setenv("ECHO_OBSERVATORY_NODE", "k8s-node-1")
	_ = os.Setenv("ECHO_OBSERVATORY_TLS", "true")
	_ = os.Setenv("ECHO_OBSERVATORY_TCP", "true")
	_ = os.Setenv("ECHO_OBSERVATORY_GRPC", "true")
	_ = os.Setenv("ECHO_OBSERVATORY_QUIC", "true")
	_ = os.Setenv("ECHO_OBSERVATORY_PRINT_HTTP_REQUEST_HEADERS", "true")
	_ = os.Setenv("ECHO_OBSERVATORY_LOG_LEVEL", "debug")
	_ = os.Setenv("ECHO_OBSERVATORY_MAX_REQUEST_SIZE", "20971520") // 20MB

	defer func() {
		_ = os.Unsetenv("ECHO_OBSERVATORY_MESSAGE")
		_ = os.Unsetenv("ECHO_OBSERVATORY_NODE")
		_ = os.Unsetenv("ECHO_OBSERVATORY_TLS")
		_ = os.Unsetenv("ECHO_OBSERVATORY_TCP")
		_ = os.Unsetenv("ECHO_OBSERVATORY_GRPC")
		_ = os.Unsetenv("ECHO_OBSERVATORY_QUIC")
		_ = os.Unsetenv("ECHO_OBSERVATORY_PRINT_HTTP_REQUEST_HEADERS")
		_ = os.Unsetenv("ECHO_OBSERVATORY_LOG_LEVEL")
		_ = os.Unsetenv("ECHO_OBSERVATORY_MAX_REQUEST_SIZE")
	}()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify all values
	assert.Equal(t, "prod-env", cfg.Message)
	assert.Equal(t, "k8s-node-1", cfg.Node)
	assert.True(t, cfg.TLS)
	assert.True(t, cfg.TCP)
	assert.True(t, cfg.GRPC)
	assert.True(t, cfg.QUIC)
	assert.True(t, cfg.PrintHeaders)
	assert.Equal(t, logrus.DebugLevel, cfg.LogLevel)
	assert.Equal(t, int64(20971520), cfg.MaxRequestSize)
}

func TestLoad_ExternalReadinessProbeConfiguration(t *testing.T) {
	viper.Reset()
	_ = os.Setenv("ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TYPE", "http")
	_ = os.Setenv("ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TARGET", "http://upstream/health")
	_ = os.Setenv("ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_INTERVAL", "5s")
	_ = os.Setenv("ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TIMEOUT", "500ms")
	_ = os.Setenv("ECHO_OBSERVATORY_EXTERNAL_READINESS_HTTP_METHOD", "HEAD")
	_ = os.Setenv("ECHO_OBSERVATORY_EXTERNAL_READINESS_HTTP_EXPECTED_STATUS", "204")
	defer func() {
		_ = os.Unsetenv("ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TYPE")
		_ = os.Unsetenv("ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TARGET")
		_ = os.Unsetenv("ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_INTERVAL")
		_ = os.Unsetenv("ECHO_OBSERVATORY_EXTERNAL_READINESS_PROBE_TIMEOUT")
		_ = os.Unsetenv("ECHO_OBSERVATORY_EXTERNAL_READINESS_HTTP_METHOD")
		_ = os.Unsetenv("ECHO_OBSERVATORY_EXTERNAL_READINESS_HTTP_EXPECTED_STATUS")
	}()

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "http", cfg.ExternalReadinessProbe.Type)
	assert.Equal(t, "http://upstream/health", cfg.ExternalReadinessProbe.Target)
	assert.Equal(t, 5*time.Second, cfg.ExternalReadinessProbe.Interval)
	assert.Equal(t, 500*time.Millisecond, cfg.ExternalReadinessProbe.Timeout)
	assert.Equal(t, "HEAD", cfg.ExternalReadinessProbe.HTTPMethod)
	assert.Equal(t, 204, cfg.ExternalReadinessProbe.HTTPExpectedStatus)
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
	}{
		{"zero body limit", map[string]string{"MAX_REQUEST_SIZE": "0"}},
		{"negative body limit", map[string]string{"MAX_REQUEST_SIZE": "-1"}},
		{"invalid port", map[string]string{"HTTP_PORT": "65536"}},
		{"conflicting TCP ports", map[string]string{"TLS": "true", "TLS_PORT": "8080"}},
		{"missing probe target", map[string]string{"EXTERNAL_READINESS_PROBE_TYPE": "tcp"}},
		{"missing probe type", map[string]string{"EXTERNAL_READINESS_PROBE_TARGET": "localhost:80"}},
		{"invalid probe type", map[string]string{"EXTERNAL_READINESS_PROBE_TYPE": "other", "EXTERNAL_READINESS_PROBE_TARGET": "localhost"}},
		{"invalid HTTP target", map[string]string{"EXTERNAL_READINESS_PROBE_TYPE": "http", "EXTERNAL_READINESS_PROBE_TARGET": "file:///etc/passwd"}},
		{"invalid TCP target", map[string]string{"EXTERNAL_READINESS_PROBE_TYPE": "tcp", "EXTERNAL_READINESS_PROBE_TARGET": "localhost"}},
		{"invalid probe interval", map[string]string{"EXTERNAL_READINESS_PROBE_TYPE": "http", "EXTERNAL_READINESS_PROBE_TARGET": "https://example.com", "EXTERNAL_READINESS_PROBE_INTERVAL": "0s"}},
		{"invalid HTTP method", map[string]string{"EXTERNAL_READINESS_PROBE_TYPE": "http", "EXTERNAL_READINESS_PROBE_TARGET": "https://example.com", "EXTERNAL_READINESS_HTTP_METHOD": "BAD METHOD"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			for name, value := range tc.env {
				t.Setenv("ECHO_OBSERVATORY_"+name, value)
			}
			cfg, err := Load()
			require.Error(t, err)
			assert.Nil(t, cfg)
		})
	}
}

func TestLoadAllowsSharedUDPPort(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ECHO_OBSERVATORY_QUIC", "true")
	t.Setenv("ECHO_OBSERVATORY_QUIC_PORT", "8080")
	t.Setenv("ECHO_OBSERVATORY_UNSAFE_HEADERS", "true")
	cfg, err := Load()
	require.NoError(t, err)
	assert.True(t, cfg.UnsafeHeaders)
}

func TestValidateTLSSettings(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Config)
	}{
		{"missing key", func(c *Config) { c.TLS = true; c.TLSCertFile = "server.pem" }},
		{"missing certificate", func(c *Config) { c.TLS = true; c.TLSKeyFile = "server.key" }},
		{"unused certificates", func(c *Config) { c.TLSCertFile = "server.pem"; c.TLSKeyFile = "server.key" }},
		{"unused CA", func(c *Config) { c.TLSClientCAFile = "ca.pem" }},
		{"gRPC TLS without gRPC", func(c *Config) { c.GRPCTLS = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			viper.Reset()
			cfg, err := Load()
			require.NoError(t, err)
			test.change(cfg)
			require.Error(t, cfg.Validate())
		})
	}
}

func TestUIEnvironmentAndFlagPrecedence(t *testing.T) {
	for _, test := range []struct {
		name, environment, flag string
		want                    bool
	}{
		{"environment opt out", "false", "", false},
		{"environment enabled", "true", "", true},
		{"flag disables environment", "true", "--ui=false", false},
		{"flag enables environment", "false", "--ui=true", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			t.Setenv("ECHO_OBSERVATORY_UI", test.environment)
			flags := pflag.NewFlagSet("ui", pflag.ContinueOnError)
			flags.Bool("ui", true, "")
			if test.flag != "" {
				require.NoError(t, flags.Parse([]string{test.flag}))
			}
			require.NoError(t, viper.BindPFlags(flags))
			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, test.want, cfg.UI)
		})
	}
}

func TestPublicURLEnvironmentAndFlagPrecedence(t *testing.T) {
	for _, test := range []struct{ name, environment, flag, want string }{
		{"environment", "https://echo.container.education", "", "https://echo.container.education/"},
		{"flag overrides environment", "https://old.example", "--public-url=https://echo.example/demo", "https://echo.example/demo/"},
		{"flag disables discovery", "https://echo.example", "--public-url=", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			t.Setenv("ECHO_OBSERVATORY_PUBLIC_URL", test.environment)
			flags := pflag.NewFlagSet("public-url", pflag.ContinueOnError)
			flags.String("public-url", "", "")
			if test.flag != "" {
				require.NoError(t, flags.Parse([]string{test.flag}))
			}
			require.NoError(t, viper.BindPFlags(flags))
			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, test.want, cfg.PublicURL)
		})
	}
}

func TestValidatePublicURL(t *testing.T) {
	for _, value := range []string{
		"", "https://echo.container.education", "https://echo.example/", "http://localhost:18880",
		"http://127.0.0.1:8080/", "https://[2001:db8::1]:8443/", "http://[::1]/demo/v2/",
		"https://echo.example/demo-v2", "https://xn--bcher-kva.example/",
	} {
		t.Run("valid/"+value, func(t *testing.T) {
			cfg := &Config{HTTPPort: "8080", MaxRequestSize: 1024, PublicURL: value}
			require.NoError(t, cfg.Validate())
			want := value
			if want != "" && !strings.HasSuffix(want, "/") {
				want += "/"
			}
			assert.Equal(t, want, cfg.PublicURL)
			require.NoError(t, cfg.Validate(), "normalization must be idempotent")
		})
	}
	for _, value := range []string{
		"/ui/", "//echo.example", "ftp://echo.example", "https:echo.example", "https:///path",
		"https://user:secret@echo.example", "https://echo.example?", "https://echo.example/?secret=x",
		"https://echo.example/#fragment", "https://echo.example/#", " https://echo.example", "https://echo.example/\nSitemap: https://evil.example/",
		"https://echo.example/with space/", "https://echo.example/with\u00a0space/", "https://bad_host/", "https://-bad.example/",
		"https://echo.example:0/", "https://echo.example:65536/", "https://echo.example:abc/", "https://echo.example:/",
		"https://::1/", "https://[not-ip]/", "https://[127.0.0.1]/", "https://[fe80::1%25en0]/",
		"https://echo.example/a/../", "https://echo.example/./", "https://echo.example//", "https://echo.example//demo/", "https://echo.example/demo//",
		"https://echo.example/%2e%2e/", "https://echo.example/a%2fb/", "https://echo.example/a\\b/", "https://echo.example/a%5Cb/", "https://echo.example/%00/",
	} {
		t.Run("invalid/"+value, func(t *testing.T) {
			cfg := &Config{HTTPPort: "8080", MaxRequestSize: 1024, PublicURL: value}
			require.ErrorContains(t, cfg.Validate(), "public URL")
		})
	}
}

func TestLoadACMEAndQUICHostEnvironment(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	for name, value := range map[string]string{
		"TLS": "true", "ACME_DOMAIN": "echo.container.education", "ACME_CACHE_DIR": "/data/acme",
		"ACME_ACCEPT_TOS": "true", "ACME_STAGING": "true", "QUIC": "true", "QUIC_HOST": "quic-bind.example", "GRPC": "true", "GRPC_TLS": "true",
	} {
		t.Setenv("ECHO_OBSERVATORY_"+name, value)
	}
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "echo.container.education", cfg.ACMEDomain)
	assert.Equal(t, "/data/acme", cfg.ACMECacheDir)
	assert.True(t, cfg.ACMEAcceptTOS)
	assert.True(t, cfg.ACMEStaging)
	assert.Equal(t, "quic-bind.example", cfg.QUICHost)
	assert.True(t, cfg.GRPCTLS)
}

func TestValidateACMESettings(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Config)
	}{
		{"cache without domain", func(c *Config) { c.ACMEDomain = ""; c.ACMEAcceptTOS = false }},
		{"terms without domain", func(c *Config) { c.ACMEDomain = ""; c.ACMECacheDir = "" }},
		{"staging without domain", func(c *Config) { c.ACMEDomain = ""; c.ACMECacheDir = ""; c.ACMEAcceptTOS = false; c.ACMEStaging = true }},
		{"HTTPS disabled", func(c *Config) { c.TLS = false }},
		{"only QUIC", func(c *Config) { c.TLS = false; c.QUIC = true }},
		{"only gRPC TLS", func(c *Config) { c.TLS = false; c.GRPC = true; c.GRPCTLS = true }},
		{"missing cache", func(c *Config) { c.ACMECacheDir = "" }},
		{"empty cache", func(c *Config) { c.ACMECacheDir = " \t" }},
		{"unaccepted terms", func(c *Config) { c.ACMEAcceptTOS = false }},
		{"static certificate", func(c *Config) { c.TLSCertFile = "server.pem"; c.TLSKeyFile = "server.key" }},
		{"client CA", func(c *Config) { c.TLSClientCAFile = "clients.pem" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			cfg, err := Load()
			require.NoError(t, err)
			cfg.TLS, cfg.ACMEDomain, cfg.ACMECacheDir, cfg.ACMEAcceptTOS = true, "echo.container.education", "/data/acme", true
			test.change(cfg)
			require.Error(t, cfg.Validate())
		})
	}
}

func TestValidateACMEDomain(t *testing.T) {
	for _, domain := range []string{
		"localhost", "echo.localhost", "*.example.com", "127.0.0.1", "::1", "https://example.com", "example.com:443",
		"example.com/path", "example.com.", "Example.com", "example..com", ".example.com", "-echo.example.com",
		"echo-.example.com", "echo_observatory.example.com", " example.com", "example.com\n", "écho.example.com",
		strings.Repeat("a", 64) + ".example.com", strings.Repeat(strings.Repeat("a", 63)+".", 4) + "com",
	} {
		t.Run(domain, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			cfg, err := Load()
			require.NoError(t, err)
			cfg.TLS, cfg.ACMEDomain, cfg.ACMECacheDir, cfg.ACMEAcceptTOS = true, domain, "/data/acme", true
			require.Error(t, cfg.Validate())
		})
	}
	for _, domain := range []string{"echo.container.education", "echo-1.example.com", "xn--bcher-kva.example"} {
		t.Run(domain, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			cfg, err := Load()
			require.NoError(t, err)
			cfg.TLS, cfg.ACMEDomain, cfg.ACMECacheDir, cfg.ACMEAcceptTOS = true, domain, "/data/acme", true
			require.NoError(t, cfg.Validate())
		})
	}
}

func TestValidateQUICHost(t *testing.T) {
	for _, test := range []struct {
		host  string
		valid bool
	}{
		{"", true}, {"quic-bind.example", true}, {"localhost", true}, {"0.0.0.0", true}, {"::", true},
		{"2001:db8::1", true}, {"fe80::1%eth0", true}, {"fe80::1%eth 0", false}, {"fe80::1%eth0\n", false}, {"quic.example.com", true}, {"QUIC.example.com.", true}, {"[::1]", false}, {"127.0.0.1:4433", false},
		{"quic.example.com:4433", false}, {"https://example.com", false}, {"a/b", false}, {" ", false},
		{"quic-bind.example\n", false}, {"bad_host", false}, {"-bad", false}, {"bad-", false},
	} {
		t.Run(test.host, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			cfg, err := Load()
			require.NoError(t, err)
			cfg.QUIC, cfg.QUICHost = true, test.host
			if test.valid {
				require.NoError(t, cfg.Validate())
			} else {
				require.Error(t, cfg.Validate())
			}
		})
	}
	viper.Reset()
	t.Cleanup(viper.Reset)
	cfg, err := Load()
	require.NoError(t, err)
	cfg.QUICHost = "quic-bind.example"
	require.Error(t, cfg.Validate(), "a QUIC bind host must not silently enable QUIC")
}

func TestLoadQUICAdvertisement(t *testing.T) {
	for _, tc := range []struct {
		name, tls, quic, port string
		valid                 bool
	}{
		{"standard mapping", "true", "true", "443", true},
		{"custom mapping", "true", "true", "19443", true},
		{"no advertisement", "false", "false", "", true},
		{"requires HTTPS", "false", "true", "443", false},
		{"requires QUIC", "true", "false", "443", false},
		{"zero", "true", "true", "0", false},
		{"noncanonical", "true", "true", "+443", false},
		{"out of range", "true", "true", "65536", false},
		{"header injection", "true", "true", "443\r\nX-Test: injected", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			t.Setenv("ECHO_OBSERVATORY_TLS", tc.tls)
			t.Setenv("ECHO_OBSERVATORY_QUIC", tc.quic)
			t.Setenv("ECHO_OBSERVATORY_QUIC_ADVERTISE_PORT", tc.port)
			cfg, err := Load()
			if tc.valid {
				require.NoError(t, err)
				assert.Equal(t, tc.port, cfg.QUICAdvertisePort)
			} else {
				require.Error(t, err)
			}
		})
	}
}
