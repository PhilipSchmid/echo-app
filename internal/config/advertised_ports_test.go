package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestLoadAdvertisedPorts(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	for _, protocol := range []string{"TLS", "TCP", "GRPC"} {
		t.Setenv("ECHO_OBSERVATORY_"+protocol, "true")
	}
	for protocol, port := range map[string]string{"HTTP": "80", "TLS": "443", "TCP": "19090", "GRPC": "15051"} {
		t.Setenv("ECHO_OBSERVATORY_"+protocol+"_ADVERTISE_PORT", port)
	}
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "80", cfg.HTTPAdvertisePort)
	require.Equal(t, "443", cfg.TLSAdvertisePort)
	require.Equal(t, "19090", cfg.TCPAdvertisePort)
	require.Equal(t, "15051", cfg.GRPCAdvertisePort)
	require.Equal(t, "8080", cfg.HTTPPort)
	require.Equal(t, "8443", cfg.TLSPort)
	require.Equal(t, "9090", cfg.TCPPort)
	require.Equal(t, "50051", cfg.GRPCPort)
}

func TestValidateAdvertisedPorts(t *testing.T) {
	for _, protocol := range []string{"HTTP", "TLS", "TCP", "gRPC"} {
		for _, port := range []string{"", "1", "443", "65535", "0", "65536", "0443", "+443", "443 ", "host:443", "443;id"} {
			t.Run(protocol+"/"+port, func(t *testing.T) {
				cfg := &Config{HTTPPort: "8080", MaxRequestSize: 1024, TLS: true, TLSPort: "8443", TCP: true, TCPPort: "9090", GRPC: true, GRPCPort: "50051"}
				switch protocol {
				case "HTTP":
					cfg.HTTPAdvertisePort = port
				case "TLS":
					cfg.TLSAdvertisePort = port
				case "TCP":
					cfg.TCPAdvertisePort = port
				case "gRPC":
					cfg.GRPCAdvertisePort = port
				}
				err := cfg.Validate()
				if port == "" || port == "1" || port == "443" || port == "65535" {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "advertisement")
				}
			})
		}
	}
	for _, cfg := range []*Config{{TLSAdvertisePort: "443"}, {TCPAdvertisePort: "9090"}, {GRPCAdvertisePort: "50051"}} {
		cfg.HTTPPort, cfg.MaxRequestSize = "8080", 1024
		require.ErrorContains(t, cfg.Validate(), "enabled listener")
	}
}
