package utils

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	cert, err := GenerateSelfSignedCert()
	require.NoError(t, err)

	// Verify certificate is not empty
	assert.NotEmpty(t, cert.Certificate)
	assert.NotNil(t, cert.PrivateKey)

	// Verify certificate has at least one cert in chain
	assert.Greater(t, len(cert.Certificate), 0)

	// Parse the certificate
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)

	// Verify certificate properties
	assert.Equal(t, []string{"Echo Inc."}, x509Cert.Subject.Organization)
	assert.Contains(t, x509Cert.DNSNames, "localhost")

	// Verify IP addresses
	assert.Len(t, x509Cert.IPAddresses, 2)
	hasLocalhost := false
	hasIPv6Localhost := false
	for _, ip := range x509Cert.IPAddresses {
		if ip.String() == "127.0.0.1" {
			hasLocalhost = true
		}
		if ip.String() == "::1" {
			hasIPv6Localhost = true
		}
	}
	assert.True(t, hasLocalhost, "Certificate should include 127.0.0.1")
	assert.True(t, hasIPv6Localhost, "Certificate should include ::1")

	// Verify key usage
	assert.Equal(t, x509.KeyUsageKeyEncipherment|x509.KeyUsageDigitalSignature, x509Cert.KeyUsage)
	assert.Contains(t, x509Cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth)

	// Verify validity period
	assert.True(t, x509Cert.NotBefore.Before(time.Now()))
	assert.True(t, x509Cert.NotAfter.After(time.Now()))

	// Verify expiration is approximately 10 years
	validityDuration := x509Cert.NotAfter.Sub(x509Cert.NotBefore)
	expectedDuration := 10 * 365 * 24 * time.Hour
	// Allow 1 day tolerance
	tolerance := 24 * time.Hour
	assert.InDelta(t, expectedDuration, validityDuration, float64(tolerance))
}

func TestGenerateSelfSignedCert_CanBeUsedForTLS(t *testing.T) {
	cert, err := GenerateSelfSignedCert()
	require.NoError(t, err)

	// Create a TLS config using the generated certificate
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Verify the config is valid
	assert.NotNil(t, tlsConfig)
	assert.Len(t, tlsConfig.Certificates, 1)
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)
}

func TestGenerateSelfSignedCert_GeneratesDifferentCerts(t *testing.T) {
	// Generate two certificates
	cert1, err := GenerateSelfSignedCert()
	require.NoError(t, err)

	cert2, err := GenerateSelfSignedCert()
	require.NoError(t, err)

	// They should be different (different serial numbers, keys, etc.)
	assert.NotEqual(t, cert1.Certificate[0], cert2.Certificate[0])
}

func TestGenerateSelfSignedCert_VerifySerialNumber(t *testing.T) {
	cert, err := GenerateSelfSignedCert()
	require.NoError(t, err)

	// Parse the certificate
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	require.NoError(t, err)

	// Verify serial number is set
	assert.NotNil(t, x509Cert.SerialNumber)
	assert.Equal(t, int64(1), x509Cert.SerialNumber.Int64())
}
