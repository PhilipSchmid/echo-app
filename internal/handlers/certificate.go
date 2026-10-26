package handlers

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"golang.org/x/crypto/acme"
)

// ServerCertificate contains only public metadata from the server's leaf certificate.
type ServerCertificate struct {
	Subject            string    `json:"subject"`
	Issuer             string    `json:"issuer"`
	DNSNames           []string  `json:"dns_names"`
	IPAddresses        []string  `json:"ip_addresses"`
	NotBefore          time.Time `json:"not_before"`
	NotAfter           time.Time `json:"not_after"`
	SHA256Fingerprint  string    `json:"sha256_fingerprint"`
	SerialNumber       string    `json:"serial_number"`
	PublicKeyAlgorithm string    `json:"public_key_algorithm"`
	SignatureAlgorithm string    `json:"signature_algorithm"`
	SelfSigned         bool      `json:"self_signed"`
}

// CertificateSource reads a public server certificate snapshot without selecting or issuing a certificate.
type CertificateSource func() *ServerCertificate

// TrackServerCertificate observes certificates selected by config. Call before using or cloning config.
func TrackServerCertificate(config *tls.Config) CertificateSource {
	type observedCertificate struct {
		digest [sha256.Size]byte
		info   ServerCertificate
	}
	var current atomic.Pointer[observedCertificate]
	record := func(cert *tls.Certificate) {
		if cert == nil || len(cert.Certificate) == 0 {
			return
		}
		digest := sha256.Sum256(cert.Certificate[0])
		if previous := current.Load(); previous != nil && previous.digest == digest {
			return
		}
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return
		}
		snapshot := ServerCertificate{
			Subject: leaf.Subject.String(), Issuer: leaf.Issuer.String(),
			DNSNames: slices.Clone(leaf.DNSNames), IPAddresses: make([]string, 0, len(leaf.IPAddresses)),
			NotBefore: leaf.NotBefore, NotAfter: leaf.NotAfter,
			SHA256Fingerprint:  strings.ReplaceAll(fmt.Sprintf("% X", digest), " ", ":"),
			SerialNumber:       strings.ToUpper(leaf.SerialNumber.Text(16)),
			PublicKeyAlgorithm: leaf.PublicKeyAlgorithm.String(), SignatureAlgorithm: leaf.SignatureAlgorithm.String(),
			SelfSigned: bytes.Equal(leaf.RawSubject, leaf.RawIssuer) && leaf.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature) == nil,
		}
		for _, ip := range leaf.IPAddresses {
			snapshot.IPAddresses = append(snapshot.IPAddresses, ip.String())
		}
		switch key := leaf.PublicKey.(type) {
		case *rsa.PublicKey:
			snapshot.PublicKeyAlgorithm = fmt.Sprintf("RSA %d bits", key.N.BitLen())
		case *ecdsa.PublicKey:
			snapshot.PublicKeyAlgorithm = "ECDSA " + key.Curve.Params().Name
		case ed25519.PublicKey:
			snapshot.PublicKeyAlgorithm = fmt.Sprintf("Ed25519 %d bits", len(key)*8)
		}
		current.Store(&observedCertificate{digest: digest, info: snapshot})
		metrics.RecordServerCertificate(leaf.NotBefore, leaf.NotAfter)
	}
	if config != nil {
		if len(config.Certificates) > 0 {
			record(&config.Certificates[0])
		}
		if getCertificate := config.GetCertificate; getCertificate != nil {
			config.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				cert, err := getCertificate(hello)
				if err == nil && (hello == nil || !slices.Contains(hello.SupportedProtos, acme.ALPNProto)) {
					record(cert)
				}
				return cert, err
			}
		}
	}
	return func() *ServerCertificate {
		snapshot := current.Load()
		if snapshot == nil {
			return nil
		}
		copy := snapshot.info
		copy.DNSNames = slices.Clone(snapshot.info.DNSNames)
		copy.IPAddresses = slices.Clone(snapshot.info.IPAddresses)
		return &copy
	}
}
