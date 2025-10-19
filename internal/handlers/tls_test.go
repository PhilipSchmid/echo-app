package handlers

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTLSConfig(t *testing.T) {
	config, err := GetTLSConfig()
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify config has certificates
	assert.Len(t, config.Certificates, 1)

	// Verify minimum TLS version
	assert.Equal(t, uint16(tls.VersionTLS12), config.MinVersion)

	// Verify certificate is valid
	cert := config.Certificates[0]
	assert.NotEmpty(t, cert.Certificate)
	assert.NotNil(t, cert.PrivateKey)
}

func TestGetTLSConfig_Caching(t *testing.T) {
	// Get TLS config multiple times
	config1, err := GetTLSConfig()
	require.NoError(t, err)

	config2, err := GetTLSConfig()
	require.NoError(t, err)

	// The certificates should be the same (cached via sync.Once)
	assert.Equal(t, config1.Certificates[0].Certificate[0], config2.Certificates[0].Certificate[0])
}

func TestGetTLSConfig_MinTLSVersion(t *testing.T) {
	config, err := GetTLSConfig()
	require.NoError(t, err)

	// Verify TLS 1.2 minimum
	assert.GreaterOrEqual(t, config.MinVersion, uint16(tls.VersionTLS12))

	// Verify it's actually TLS 1.2
	assert.Equal(t, uint16(tls.VersionTLS12), config.MinVersion)
}

func TestGetTLSConfig_CertificateProperties(t *testing.T) {
	config, err := GetTLSConfig()
	require.NoError(t, err)

	cert := config.Certificates[0]

	// Verify certificate has at least one cert in chain
	assert.Greater(t, len(cert.Certificate), 0)

	// Verify private key is present
	assert.NotNil(t, cert.PrivateKey)

	// Verify leaf certificate (if parsed)
	if cert.Leaf != nil {
		assert.NotEmpty(t, cert.Leaf.Subject.Organization)
		assert.Contains(t, cert.Leaf.DNSNames, "localhost")
	}
}
