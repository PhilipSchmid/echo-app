package handlers

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/utils"
)

var (
	tlsCert     tls.Certificate
	tlsCertOnce sync.Once
	tlsCertErr  error
)

// GetTLSConfig loads explicit trust material or uses the cached demo certificate.
func GetTLSConfig(configs ...*config.Config) (*tls.Config, error) {
	var cfg config.Config
	if len(configs) > 0 && configs[0] != nil {
		cfg = *configs[0]
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return nil, fmt.Errorf("TLS certificate and key files must be supplied together")
	}
	var cert tls.Certificate
	var err error
	if cfg.TLSCertFile != "" {
		cert, err = tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	} else {
		tlsCertOnce.Do(func() { tlsCert, tlsCertErr = utils.GenerateSelfSignedCert() })
		cert, err = tlsCert, tlsCertErr
	}
	if err != nil {
		return nil, fmt.Errorf("load server certificate: %w", err)
	}
	result := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	if cfg.TLSClientCAFile != "" {
		data, err := os.ReadFile(cfg.TLSClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("read client CA: %w", err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(data) {
			return nil, fmt.Errorf("client CA file contains no valid PEM certificates")
		}
		result.ClientCAs = roots
		result.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return result, nil
}
