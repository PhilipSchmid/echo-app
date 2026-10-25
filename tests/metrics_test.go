//go:build integration

package integration_test

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// Check the real private scrape after traffic over every protocol, including
// the live WebSocket sessions and readiness dependency exercised by the suite.
func checkProtocolMetrics(t *testing.T, client *http.Client, adminURL, host string, ports map[string]string) {
	t.Helper()
	response, err := client.Post("http://"+net.JoinHostPort(host, ports["http"])+"/", "text/plain", strings.NewReader(strings.Repeat("a", integrationMaxRequestSize+1)))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected body-limit response, got %d", response.StatusCode)
	}
	response, err = client.Get(adminURL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(response.Body)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("parse live scrape: status=%d err=%v", response.StatusCode, err)
	}
	atLeast := func(name string, labels map[string]string, minimum float64) {
		t.Helper()
		for _, sample := range families[name].GetMetric() {
			matched := true
			for key, value := range labels {
				found := false
				for _, label := range sample.GetLabel() {
					if label.GetName() == key && label.GetValue() == value {
						found = true
					}
				}
				matched = matched && found
			}
			if matched && sample.GetCounter().GetValue()+sample.GetGauge().GetValue() >= minimum {
				return
			}
		}
		t.Errorf("missing live metric %s%v >= %g", name, labels, minimum)
	}
	for _, listener := range []string{"H2C", "TLS", "TCP", "gRPC", "QUIC", "Metrics"} {
		atLeast("echo_observatory_requests_total", map[string]string{"listener": listener}, 1)
		atLeast("echo_observatory_connections_total", map[string]string{"listener": listener}, 1)
		atLeast("echo_observatory_listener_up", map[string]string{"listener": listener}, 1)
	}
	for _, listener := range []string{"H2C", "TLS", "QUIC"} {
		atLeast("echo_observatory_requests_total", map[string]string{"listener": listener, "endpoint": "/ui/"}, 1)
	}
	for _, code := range []string{"400", "413", "418", "599"} {
		atLeast("echo_observatory_http_responses_total", map[string]string{"listener": "H2C", "endpoint": "/", "status_code": code}, 1)
	}
	atLeast("echo_observatory_errors_total", map[string]string{"listener": "H2C", "error_type": "request_too_large"}, 1)
	atLeast("echo_observatory_http_responses_total", map[string]string{"listener": "Metrics", "endpoint": "/health", "status_code": "200"}, 1)
	for _, tc := range []struct{ listener, protocol string }{
		{"H2C", "HTTP/1.1"}, {"H2C", "HTTP/2"}, {"TLS", "HTTP/1.1"}, {"TLS", "HTTP/2"}, {"QUIC", "HTTP/3"},
	} {
		atLeast("echo_observatory_http_protocol_requests_total", map[string]string{"listener": tc.listener, "protocol": tc.protocol, "ip_version": expectedIPVersion(host)}, 1)
	}
	for _, method := range []string{"/echo.EchoService/Echo", "/grpc.health.v1.Health/Check"} {
		atLeast("echo_observatory_grpc_responses_total", map[string]string{"method": method, "code": "OK"}, 1)
	}
	for _, direction := range []string{"received", "sent"} {
		for _, kind := range []string{"text", "binary"} {
			labels := map[string]string{"direction": direction, "message_type": kind}
			atLeast("echo_observatory_websocket_messages_total", labels, 2)
			atLeast("echo_observatory_websocket_message_bytes_total", labels, 8)
		}
	}
	for _, name := range []string{"healthy", "ready", "external_probe_ready"} {
		atLeast("echo_observatory_"+name, nil, 1)
	}
	for _, result := range []string{"success", "failure"} {
		atLeast("echo_observatory_external_probe_checks_total", map[string]string{"result": result}, 1)
	}
	for _, family := range families {
		for _, sample := range family.GetMetric() {
			for _, label := range sample.GetLabel() {
				if strings.Contains(label.GetValue(), host) || strings.Contains(label.GetValue(), "item=42") || strings.Contains(label.GetValue(), "/test/path") {
					t.Errorf("request-specific data leaked into metric label %s=%q", label.GetName(), label.GetValue())
				}
			}
		}
	}
}
