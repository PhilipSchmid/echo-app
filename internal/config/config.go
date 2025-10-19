package config

import (
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Config struct {
	Message        string
	Node           string
	PrintHeaders   bool
	TLS            bool
	TCP            bool
	GRPC           bool
	QUIC           bool
	Metrics        bool
	HTTPPort       string
	TLSPort        string
	TCPPort        string
	GRPCPort       string
	QUICPort       string
	MetricsPort    string
	LogLevel       logrus.Level
	MaxRequestSize int64 // Maximum request body size in bytes
}

func Load() (*Config, error) {
	viper.SetEnvPrefix("ECHO_APP")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	// Set default values
	viper.SetDefault("message", "")
	viper.SetDefault("node", "")
	viper.SetDefault("print-http-request-headers", false)
	viper.SetDefault("tls", false)
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

	// Load configuration from viper
	cfg := &Config{
		Message:        viper.GetString("message"),
		Node:           viper.GetString("node"),
		PrintHeaders:   viper.GetBool("print-http-request-headers"),
		TLS:            viper.GetBool("tls"),
		TCP:            viper.GetBool("tcp"),
		GRPC:           viper.GetBool("grpc"),
		QUIC:           viper.GetBool("quic"),
		Metrics:        viper.GetBool("metrics"),
		HTTPPort:       viper.GetString("http-port"),
		TLSPort:        viper.GetString("tls-port"),
		TCPPort:        viper.GetString("tcp-port"),
		GRPCPort:       viper.GetString("grpc-port"),
		QUICPort:       viper.GetString("quic-port"),
		MetricsPort:    viper.GetString("metrics-port"),
		MaxRequestSize: viper.GetInt64("max-request-size"),
	}

	// Set log level
	lvl, err := logrus.ParseLevel(viper.GetString("log-level"))
	if err != nil {
		return nil, err
	}
	cfg.LogLevel = lvl
	logrus.SetLevel(cfg.LogLevel)

	return cfg, nil
}
