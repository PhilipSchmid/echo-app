package metrics

import (
	"net/http"

	"github.com/PhilipSchmid/echo-app/internal/config"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// StartMetricsServer starts the Prometheus metrics server
func StartMetricsServer(cfg *config.Config) {
	http.Handle("/metrics", promhttp.Handler())
	logrus.Infof("Metrics server listening on port %s", cfg.MetricsPort)
	if err := http.ListenAndServe(":"+cfg.MetricsPort, nil); err != nil {
		logrus.Errorf("Metrics server failed: %v", err)
	}
}
