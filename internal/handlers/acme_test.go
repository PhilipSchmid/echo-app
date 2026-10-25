package handlers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

func acmeTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{ACMEDomain: "echo.example.com", ACMECacheDir: t.TempDir(), ACMEAcceptTOS: true}
}

func TestACMEManagerPolicyAndEnvironment(t *testing.T) {
	for _, staging := range []bool{false, true} {
		t.Run(fmt.Sprintf("staging=%t", staging), func(t *testing.T) {
			cfg := acmeTestConfig(t)
			cfg.ACMEStaging = staging
			manager, err := newACMEManager(cfg)
			require.NoError(t, err)
			assert.True(t, manager.Prompt("https://example.com/terms"))
			assert.NoError(t, manager.HostPolicy(context.Background(), cfg.ACMEDomain))
			assert.Error(t, manager.HostPolicy(context.Background(), "other.example.com"))
			assert.Error(t, manager.HostPolicy(context.Background(), "sub."+cfg.ACMEDomain))
			name, directory := "production", autocert.DefaultACMEDirectory
			if staging {
				name, directory = "staging", acmeStagingDirectory
			}
			assert.Equal(t, directory, manager.Client.DirectoryURL)
			assert.Equal(t, autocert.DirCache(filepath.Join(cfg.ACMECacheDir, name)), manager.Cache)
			info, err := os.Stat(filepath.Join(cfg.ACMECacheDir, name))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
			entries, err := os.ReadDir(filepath.Join(cfg.ACMECacheDir, name))
			require.NoError(t, err)
			assert.Empty(t, entries, "startup probes must not leave cache entries")
		})
	}
}

func TestACMERejectsIncompleteOrConflictingSettings(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*config.Config)
	}{
		{"terms not accepted", func(cfg *config.Config) { cfg.ACMEAcceptTOS = false }},
		{"missing domain", func(cfg *config.Config) { cfg.ACMEDomain = "" }},
		{"missing cache", func(cfg *config.Config) { cfg.ACMECacheDir = "" }},
		{"empty cache", func(cfg *config.Config) { cfg.ACMECacheDir = " \t" }},
		{"explicit certificate", func(cfg *config.Config) { cfg.TLSCertFile = "server.pem"; cfg.TLSKeyFile = "server.key" }},
		{"client authentication", func(cfg *config.Config) { cfg.TLSClientCAFile = "clients.pem" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := acmeTestConfig(t)
			test.change(cfg)
			_, err := newACMEManager(cfg)
			require.Error(t, err)
		})
	}
}

func TestACMECacheFailsBeforeIssuance(t *testing.T) {
	for _, kind := range []string{"file", "public", "unwritable", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			cfg := acmeTestConfig(t)
			dir := filepath.Join(cfg.ACMECacheDir, "production")
			switch kind {
			case "file":
				require.NoError(t, os.WriteFile(dir, []byte("not a directory"), 0600))
			case "public":
				require.NoError(t, os.Mkdir(dir, 0755))
				require.NoError(t, os.Chmod(dir, 0755))
			case "unwritable":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses directory write permissions")
				}
				require.NoError(t, os.Mkdir(dir, 0500))
				t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0700)) })
			case "symlink":
				require.NoError(t, os.Symlink(t.TempDir(), dir))
			}
			_, err := newACMEManager(cfg)
			require.Error(t, err)
		})
	}
}

type denyACMERequests struct{ calls int }

func (d *denyACMERequests) RoundTrip(*http.Request) (*http.Response, error) {
	d.calls++
	return nil, fmt.Errorf("unexpected ACME network request")
}

func cachedACMECertificate(t *testing.T, domain string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	now := time.Now()
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(90 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	require.NoError(t, err)
	private, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	return append(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: private}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
}

func TestACMECachedCertificateAndSharedClones(t *testing.T) {
	cfg := acmeTestConfig(t)
	manager, err := newACMEManager(cfg)
	require.NoError(t, err)
	transport := &denyACMERequests{}
	manager.Client.HTTPClient = &http.Client{Transport: transport}
	data := cachedACMECertificate(t, cfg.ACMEDomain)
	require.NoError(t, manager.Cache.Put(context.Background(), cfg.ACMEDomain, data))
	require.NoError(t, manager.Cache.Put(context.Background(), "other.example.com", cachedACMECertificate(t, "other.example.com")))
	hello := &tls.ClientHelloInfo{ServerName: cfg.ACMEDomain, CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}, SupportedProtos: []string{"h2"}}
	shared := manager.TLSConfig()
	assert.Contains(t, shared.NextProtos, acme.ALPNProto)
	first, err := shared.GetCertificate(hello)
	require.NoError(t, err)
	require.NoError(t, first.Leaf.VerifyHostname(cfg.ACMEDomain))
	require.NoError(t, manager.Cache.Delete(context.Background(), cfg.ACMEDomain))
	clone := shared.Clone()
	clone.NextProtos = []string{"h3"}
	second, err := clone.GetCertificate(hello)
	require.NoError(t, err)
	assert.Equal(t, first.Certificate, second.Certificate)
	assert.Same(t, first.PrivateKey, second.PrivateKey, "clones must share the manager's in-memory state")
	assert.Contains(t, shared.NextProtos, acme.ALPNProto)
	_, err = clone.GetCertificate(&tls.ClientHelloInfo{ServerName: "other.example.com"})
	require.Error(t, err, "even cached certificates must obey the host allowlist")
	assert.Zero(t, transport.calls)
}

func TestGetTLSConfigACME(t *testing.T) {
	cfg := acmeTestConfig(t)
	managed, err := GetTLSConfig(cfg)
	require.NoError(t, err)
	assert.Empty(t, managed.Certificates, "ACME must never silently fall back to a demo certificate")
	require.NotNil(t, managed.GetCertificate)
	assert.Contains(t, managed.NextProtos, acme.ALPNProto)
	assert.Equal(t, uint16(tls.VersionTLS12), managed.MinVersion)
	_, err = managed.GetCertificate(&tls.ClientHelloInfo{ServerName: "other.example.com"})
	require.Error(t, err)
}
