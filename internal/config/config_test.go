package config

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
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
	assert.False(t, cfg.TLS)
	assert.False(t, cfg.TCP)
	assert.False(t, cfg.GRPC)
	assert.False(t, cfg.QUIC)
	assert.True(t, cfg.Metrics)
	assert.Equal(t, "8080", cfg.HTTPPort)
	assert.Equal(t, "8443", cfg.TLSPort)
	assert.Equal(t, "9090", cfg.TCPPort)
	assert.Equal(t, "50051", cfg.GRPCPort)
	assert.Equal(t, "4433", cfg.QUICPort)
	assert.Equal(t, "3000", cfg.MetricsPort)
	assert.Equal(t, int64(10485760), cfg.MaxRequestSize) // 10MB
	assert.Equal(t, logrus.InfoLevel, cfg.LogLevel)
}

func TestLoad_EnvironmentVariables(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(t *testing.T, cfg *Config)
	}{
		{
			name: "custom message",
			envVars: map[string]string{
				"ECHO_APP_MESSAGE": "test-message",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "test-message", cfg.Message)
			},
		},
		{
			name: "custom node",
			envVars: map[string]string{
				"ECHO_APP_NODE": "test-node",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "test-node", cfg.Node)
			},
		},
		{
			name: "enable TLS",
			envVars: map[string]string{
				"ECHO_APP_TLS": "true",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.TLS)
			},
		},
		{
			name: "enable TCP",
			envVars: map[string]string{
				"ECHO_APP_TCP": "true",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.TCP)
			},
		},
		{
			name: "enable GRPC",
			envVars: map[string]string{
				"ECHO_APP_GRPC": "true",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.GRPC)
			},
		},
		{
			name: "enable QUIC",
			envVars: map[string]string{
				"ECHO_APP_QUIC": "true",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.QUIC)
			},
		},
		{
			name: "print headers",
			envVars: map[string]string{
				"ECHO_APP_PRINT_HTTP_REQUEST_HEADERS": "true",
			},
			validate: func(t *testing.T, cfg *Config) {
				assert.True(t, cfg.PrintHeaders)
			},
		},
		{
			name: "custom ports",
			envVars: map[string]string{
				"ECHO_APP_HTTP_PORT":    "9000",
				"ECHO_APP_TLS_PORT":     "9443",
				"ECHO_APP_TCP_PORT":     "9999",
				"ECHO_APP_GRPC_PORT":    "50052",
				"ECHO_APP_QUIC_PORT":    "4444",
				"ECHO_APP_METRICS_PORT": "3001",
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
				"ECHO_APP_MAX_REQUEST_SIZE": "5242880", // 5MB
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
				os.Setenv(key, value)
				defer os.Unsetenv(key)
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

			os.Setenv("ECHO_APP_MESSAGE", tt.message)
			defer os.Unsetenv("ECHO_APP_MESSAGE")

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
				os.Setenv("ECHO_APP_LOG_LEVEL", tt.logLevel)
				defer os.Unsetenv("ECHO_APP_LOG_LEVEL")
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
	os.Setenv("ECHO_APP_MESSAGE", "prod-env")
	os.Setenv("ECHO_APP_NODE", "k8s-node-1")
	os.Setenv("ECHO_APP_TLS", "true")
	os.Setenv("ECHO_APP_TCP", "true")
	os.Setenv("ECHO_APP_GRPC", "true")
	os.Setenv("ECHO_APP_QUIC", "true")
	os.Setenv("ECHO_APP_PRINT_HTTP_REQUEST_HEADERS", "true")
	os.Setenv("ECHO_APP_LOG_LEVEL", "debug")
	os.Setenv("ECHO_APP_MAX_REQUEST_SIZE", "20971520") // 20MB

	defer func() {
		os.Unsetenv("ECHO_APP_MESSAGE")
		os.Unsetenv("ECHO_APP_NODE")
		os.Unsetenv("ECHO_APP_TLS")
		os.Unsetenv("ECHO_APP_TCP")
		os.Unsetenv("ECHO_APP_GRPC")
		os.Unsetenv("ECHO_APP_QUIC")
		os.Unsetenv("ECHO_APP_PRINT_HTTP_REQUEST_HEADERS")
		os.Unsetenv("ECHO_APP_LOG_LEVEL")
		os.Unsetenv("ECHO_APP_MAX_REQUEST_SIZE")
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
