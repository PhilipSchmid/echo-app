package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ServerCertificateNotBefore = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "echo_observatory_tls_certificate_not_before_timestamp_seconds",
		Help: "Unix start of validity of the last observed ordinary server leaf certificate shared by TLS listeners; absent until observed, excludes ACME challenge certificates.",
	}, nil)
	ServerCertificateNotAfter = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "echo_observatory_tls_certificate_not_after_timestamp_seconds",
		Help: "Unix expiry of the last observed ordinary server leaf certificate shared by TLS listeners; absent until observed, excludes ACME challenge certificates.",
	}, nil)
)

// RecordServerCertificate exposes dates only; certificate identities never become labels.
func RecordServerCertificate(notBefore, notAfter time.Time) {
	ServerCertificateNotBefore.WithLabelValues().Set(float64(notBefore.Unix()))
	ServerCertificateNotAfter.WithLabelValues().Set(float64(notAfter.Unix()))
}
