package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/require"
)

func TestPublicConfigUsesAdvertisedPortsAndOmitsAdmin(t *testing.T) {
	for _, advertised := range []bool{false, true} {
		cfg := &config.Config{HTTPPort: "8080", TLSPort: "8443", TCPPort: "9090", GRPCPort: "50051", QUICPort: "4433", Metrics: true, MetricsPort: "private-admin-port"}
		expected := map[string]string{"http_port": "8080", "tls_port": "8443", "tcp_port": "9090", "grpc_port": "50051", "quic_port": "4433"}
		if advertised {
			cfg.HTTPAdvertisePort, cfg.TLSAdvertisePort, cfg.TCPAdvertisePort, cfg.GRPCAdvertisePort, cfg.QUICAdvertisePort = "80", "443", "19090", "15051", "443"
			expected = map[string]string{"http_port": "80", "tls_port": "443", "tcp_port": "19090", "grpc_port": "15051", "quic_port": "443"}
		}
		response := httptest.NewRecorder()
		NewHandler(cfg, "HTTP").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/config.json", nil))
		require.Equal(t, http.StatusOK, response.Code)
		var settings map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &settings))
		for name, port := range expected {
			require.Equal(t, port, settings[name], name)
		}
		require.NotContains(t, settings, "metrics")
		require.NotContains(t, settings, "metrics_port")
		require.NotContains(t, response.Body.String(), cfg.MetricsPort)
	}
}

func TestCertificateInsightsLiveInsideRequestSecurity(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		handler := NewHandler(&config.Config{TLS: enabled, HTTPPort: "8080"}, "HTTP")
		request := httptest.NewRecorder()
		handler.ServeHTTP(request, httptest.NewRequest(http.MethodGet, "/ui/", nil))
		require.Equal(t, http.StatusOK, request.Code)
		require.Contains(t, request.Body.String(), `id="connection-security"`)
		require.Contains(t, request.Body.String(), "This request used HTTP")
		if enabled {
			require.Contains(t, request.Body.String(), `id="certificate-help"`)
		} else {
			require.NotContains(t, request.Body.String(), `id="certificate-help"`)
		}
		connect := httptest.NewRecorder()
		handler.ServeHTTP(connect, httptest.NewRequest(http.MethodGet, "/ui/connect/", nil))
		require.Equal(t, http.StatusOK, connect.Code)
		for _, private := range []string{`id="certificate-fields"`, `id="certificate-help"`, `id="health-metrics"`, `id="admin-cards"`} {
			require.NotContains(t, connect.Body.String(), private)
		}
	}
}
