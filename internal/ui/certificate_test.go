package ui

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCertificateEndpointDescribesOnlyPublicServerMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		cfg        config.Config
	}{
		{"disabled", "disabled", config.Config{}},
		{"demo", "demo", config.Config{TLS: true}},
		{"quic only", "demo", config.Config{QUIC: true}},
		{"grpc TLS only", "demo", config.Config{GRPC: true, GRPCTLS: true}},
		{"file", "file", config.Config{TLS: true, TLSCertFile: "/private/server.pem"}},
		{"ACME", "acme", config.Config{TLS: true, ACMEDomain: "private-config.example"}},
		{"ACME staging", "acme-staging", config.Config{TLS: true, ACMEDomain: "private-config.example", ACMEStaging: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.TLSKeyFile, tc.cfg.TLSClientCAFile, tc.cfg.ACMECacheDir = "/private/key.pem", "/private/clients.pem", "/private/acme"
			certificate := &handlers.ServerCertificate{Subject: "CN=public.example", Issuer: "O=Public issuer", DNSNames: []string{"public.example"}, IPAddresses: []string{"::1"}, NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), SHA256Fingerprint: "AA:BB", SerialNumber: "ABC", PublicKeyAlgorithm: "RSA 2048 bits", SignatureAlgorithm: "SHA256-RSA", SelfSigned: true}
			reads := 0
			source := func() *handlers.ServerCertificate { reads++; return certificate }
			handler := NewHandler(&tc.cfg, "HTTP", source)
			request := httptest.NewRequest(http.MethodGet, "/ui/certificate.json?secret=private-request-marker", nil)
			request.Header.Set("Authorization", "private-request-marker")
			request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: "private-client-marker"}}}}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
			var document struct {
				Mode        string                      `json:"mode"`
				Certificate *handlers.ServerCertificate `json:"certificate"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &document))
			assert.Equal(t, tc.mode, document.Mode)
			if tc.mode == "disabled" {
				assert.Nil(t, document.Certificate)
				assert.Zero(t, reads)
			} else {
				assert.Equal(t, certificate, document.Certificate)
				assert.Equal(t, 1, reads)
			}
			for _, private := range []string{"/private/", "private-config.example", "private-request-marker", "private-client-marker", "PRIVATE KEY"} {
				assert.NotContains(t, response.Body.String(), private)
			}
			assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			assert.Equal(t, "noindex", response.Header().Get("X-Robots-Tag"))
			before := reads
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/ui/certificate.json", nil))
			assert.Equal(t, before, reads, "rejected methods must not read the certificate source")
		})
	}
}

func TestCertificateEndpointPendingAndSnapshotRefresh(t *testing.T) {
	cfg := &config.Config{TLS: true}
	response := httptest.NewRecorder()
	NewHandler(cfg, "HTTP").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/certificate.json", nil))
	assert.JSONEq(t, `{"mode":"demo","certificate":null}`, response.Body.String())
	current := &handlers.ServerCertificate{SHA256Fingerprint: "first"}
	handler := NewHandler(cfg, "HTTP", func() *handlers.ServerCertificate { return current })
	for _, fingerprint := range []string{"first", "renewed"} {
		current = &handlers.ServerCertificate{SHA256Fingerprint: fingerprint}
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/certificate.json", nil))
		assert.Contains(t, response.Body.String(), `"sha256_fingerprint":"`+fingerprint+`"`)
	}
	current.NotAfter = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/certificate.json", nil))
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.NotContains(t, response.Body.String(), "renewed")
}
