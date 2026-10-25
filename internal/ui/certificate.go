package ui

import "github.com/PhilipSchmid/echo-observatory/v2/internal/config"

func certificateMode(cfg *config.Config) string {
	tlsEnabled := cfg.TLS || cfg.QUIC || (cfg.GRPC && cfg.GRPCTLS)
	if !tlsEnabled {
		return "disabled"
	}
	switch {
	case cfg.ACMEDomain != "" && cfg.ACMEStaging:
		return "acme-staging"
	case cfg.ACMEDomain != "":
		return "acme"
	case cfg.TLSCertFile != "":
		return "file"
	default:
		return "demo"
	}
}
