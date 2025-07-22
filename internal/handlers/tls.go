package handlers

import (
	"crypto/tls"
	"sync"

	"github.com/PhilipSchmid/echo-app/internal/utils"
)

var (
	tlsCert     tls.Certificate
	tlsCertOnce sync.Once
	tlsCertErr  error
)

// GetTLSConfig returns a TLS configuration with a cached self-signed certificate
func GetTLSConfig() (*tls.Config, error) {
	tlsCertOnce.Do(func() {
		tlsCert, tlsCertErr = utils.GenerateSelfSignedCert()
	})

	if tlsCertErr != nil {
		return nil, tlsCertErr
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
