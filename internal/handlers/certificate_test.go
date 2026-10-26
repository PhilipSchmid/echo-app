package handlers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/acme"
)

func snapshotTestCertificate(t *testing.T, serial int64, signer *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	if signer == nil {
		signer = key
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "echo.example", Organization: []string{"Echo test"}},
		DNSNames: []string{"echo.example", "www.echo.example"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, leaf, &key.PublicKey, signer)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestTrackServerCertificateStatic(t *testing.T) {
	cert := snapshotTestCertificate(t, 0x123ABC, nil)
	config := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	source := TrackServerCertificate(config)
	snapshot := source()
	require.NotNil(t, snapshot)
	assert.Equal(t, "CN=echo.example,O=Echo test", snapshot.Subject)
	assert.Equal(t, snapshot.Subject, snapshot.Issuer)
	assert.Equal(t, []string{"echo.example", "www.echo.example"}, snapshot.DNSNames)
	assert.Equal(t, []string{"127.0.0.1", "::1"}, snapshot.IPAddresses)
	assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), snapshot.NotBefore)
	assert.Equal(t, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), snapshot.NotAfter)
	assert.Equal(t, "123ABC", snapshot.SerialNumber)
	assert.Equal(t, "ECDSA P-256", snapshot.PublicKeyAlgorithm)
	assert.Equal(t, "ECDSA-SHA256", snapshot.SignatureAlgorithm)
	assert.True(t, snapshot.SelfSigned, "a self-signed leaf need not have CA permissions")
	digest := sha256.Sum256(cert.Certificate[0])
	assert.Equal(t, strings.ToUpper(hex.EncodeToString(digest[:])), strings.ReplaceAll(snapshot.SHA256Fingerprint, ":", ""))
	assert.Len(t, snapshot.SHA256Fingerprint, 95)
	assert.Nil(t, config.GetCertificate, "static TLS selection must remain unchanged")

	encoded, err := json.Marshal(snapshot)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(encoded, &fields))
	assert.Len(t, fields, 11, "only the documented public metadata is serialized")
	assert.NotContains(t, fields, "private_key")
	assert.NotContains(t, fields, "certificate")

	snapshot.DNSNames[0], snapshot.IPAddresses[0] = "changed", "changed"
	snapshot.Subject = "changed"
	assert.Equal(t, "echo.example", source().DNSNames[0])
	assert.Equal(t, "127.0.0.1", source().IPAddresses[0])
	assert.Equal(t, "CN=echo.example,O=Echo test", source().Subject)
}

func TestTrackServerCertificateChecksSelfSignature(t *testing.T) {
	signer, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	cert := snapshotTestCertificate(t, 1, signer)
	snapshot := TrackServerCertificate(&tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})()
	require.NotNil(t, snapshot)
	assert.Equal(t, snapshot.Subject, snapshot.Issuer)
	assert.False(t, snapshot.SelfSigned, "matching names alone do not prove a self-signature")
}

func TestTrackServerCertificateRenewalAndFailures(t *testing.T) {
	first, renewed := snapshotTestCertificate(t, 1, nil), snapshotTestCertificate(t, 2, nil)
	selected := &first
	var callbackError error
	var calls int
	config := &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		calls++
		return selected, callbackError
	}}
	source := TrackServerCertificate(config)
	assert.Nil(t, source())
	assert.Zero(t, calls, "installing or reading the source must never invoke the provider")
	hello := &tls.ClientHelloInfo{ServerName: "echo.example", SupportedProtos: []string{"h2"}}
	got, err := config.GetCertificate(hello)
	require.NoError(t, err)
	assert.Same(t, selected, got)
	initialFingerprint := source().SHA256Fingerprint
	allocations := testing.AllocsPerRun(10, func() {
		_, callbackErr := config.GetCertificate(hello)
		if callbackErr != nil {
			panic(callbackErr)
		}
	})
	assert.Zero(t, allocations, "selecting the same certificate must reuse its parsed metadata")
	selected = &renewed
	_, err = config.GetCertificate(hello)
	require.NoError(t, err)
	assert.NotEqual(t, initialFingerprint, source().SHA256Fingerprint)
	assert.Equal(t, "2", source().SerialNumber)

	for _, invalid := range []*tls.Certificate{nil, {}, {Certificate: [][]byte{[]byte("malformed")}}} {
		selected = invalid
		got, err = config.GetCertificate(hello)
		require.NoError(t, err)
		assert.Equal(t, selected, got)
		assert.Equal(t, "2", source().SerialNumber, "an invalid provider result must preserve the previous snapshot")
	}
	selected, callbackError = &first, errors.New("renewal failed")
	got, err = config.GetCertificate(hello)
	assert.Same(t, selected, got)
	assert.ErrorIs(t, err, callbackError)
	assert.Equal(t, "2", source().SerialNumber)
	beforeRead := calls
	_ = source()
	assert.Equal(t, beforeRead, calls)
}

func TestTrackServerCertificateSkipsACMEChallenge(t *testing.T) {
	ordinary, challenge := snapshotTestCertificate(t, 1, nil), snapshotTestCertificate(t, 2, nil)
	config := &tls.Config{Certificates: []tls.Certificate{ordinary}, MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		return &challenge, nil
	}}
	source := TrackServerCertificate(config)
	got, err := config.GetCertificate(&tls.ClientHelloInfo{SupportedProtos: []string{acme.ALPNProto}})
	require.NoError(t, err)
	assert.Same(t, &challenge, got)
	assert.Equal(t, "1", source().SerialNumber)
	_, err = config.GetCertificate(&tls.ClientHelloInfo{SupportedProtos: []string{"h2"}})
	require.NoError(t, err)
	assert.Equal(t, "2", source().SerialNumber)
}

func TestTrackServerCertificateConcurrent(t *testing.T) {
	first, second := snapshotTestCertificate(t, 1, nil), snapshotTestCertificate(t, 2, nil)
	var selected atomic.Pointer[tls.Certificate]
	selected.Store(&first)
	config := &tls.Config{Certificates: []tls.Certificate{first}, MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		return selected.Load(), nil
	}}
	source := TrackServerCertificate(config)
	var workers sync.WaitGroup
	for worker := range 8 {
		workers.Go(func() {
			for range 100 {
				if worker%2 == 0 {
					selected.Store(&second)
					_, err := config.GetCertificate(&tls.ClientHelloInfo{})
					assert.NoError(t, err)
				} else {
					snapshot := source()
					assert.Contains(t, []string{"1", "2"}, snapshot.SerialNumber)
					snapshot.DNSNames[0] = "local copy"
				}
			}
		})
	}
	workers.Wait()
}

func TestServerCertificateMetricsFollowSelectionAndRenewal(t *testing.T) {
	metrics.ServerCertificateNotBefore.Reset()
	metrics.ServerCertificateNotAfter.Reset()
	t.Cleanup(func() {
		metrics.ServerCertificateNotBefore.Reset()
		metrics.ServerCertificateNotAfter.Reset()
	})
	first := snapshotTestCertificate(t, 1, nil)
	renewed := snapshotTestCertificate(t, 2, nil)
	leaf, err := x509.ParseCertificate(renewed.Certificate[0])
	require.NoError(t, err)
	leaf.NotAfter = leaf.NotAfter.Add(30 * 24 * time.Hour)
	key := renewed.PrivateKey.(*ecdsa.PrivateKey)
	renewed.Certificate[0], err = x509.CreateCertificate(rand.Reader, leaf, leaf, &key.PublicKey, key)
	require.NoError(t, err)
	selected := &first
	var providerError error
	var calls int
	config := &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		calls++
		return selected, providerError
	}}
	source := TrackServerCertificate(config)
	registry := prometheus.NewRegistry()
	registry.MustRegister(metrics.ServerCertificateNotBefore, metrics.ServerCertificateNotAfter)
	families, err := registry.Gather()
	require.NoError(t, err)
	assert.Empty(t, families, "no certificate must not look like an expired certificate from 1970")
	assert.Zero(t, calls, "scraping must never select or issue a certificate")
	_, err = config.GetCertificate(&tls.ClientHelloInfo{SupportedProtos: []string{"h2"}})
	require.NoError(t, err)
	assert.Equal(t, float64(source().NotAfter.Unix()), testutil.ToFloat64(metrics.ServerCertificateNotAfter.WithLabelValues()))
	selected = &renewed
	_, err = config.GetCertificate(&tls.ClientHelloInfo{SupportedProtos: []string{acme.ALPNProto}})
	require.NoError(t, err)
	assert.NotEqual(t, float64(leaf.NotAfter.Unix()), testutil.ToFloat64(metrics.ServerCertificateNotAfter.WithLabelValues()), "challenge certificates must be ignored")
	_, err = config.GetCertificate(&tls.ClientHelloInfo{SupportedProtos: []string{"h2"}})
	require.NoError(t, err)
	assert.Equal(t, float64(leaf.NotAfter.Unix()), testutil.ToFloat64(metrics.ServerCertificateNotAfter.WithLabelValues()))
	assert.Equal(t, float64(leaf.NotBefore.Unix()), testutil.ToFloat64(metrics.ServerCertificateNotBefore.WithLabelValues()))
	selected, providerError = &first, errors.New("provider failed")
	_, err = config.GetCertificate(&tls.ClientHelloInfo{})
	require.Error(t, err)
	assert.Equal(t, float64(leaf.NotAfter.Unix()), testutil.ToFloat64(metrics.ServerCertificateNotAfter.WithLabelValues()))
	families, err = registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 2)
	for _, family := range families {
		assert.Empty(t, family.Metric[0].Label, "dates must have no domain or certificate identity labels")
	}
}
