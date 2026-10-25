package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfiguredResponseControlCap(t *testing.T) {
	for _, tc := range []struct {
		name, method, query string
		status, bodyBytes   int
		contentLength       string
	}{
		{"exact cap", http.MethodGet, "size=256", http.StatusOK, 256, "256"},
		{"head at cap", http.MethodHead, "size=256", http.StatusOK, 0, "256"},
		{"above cap", http.MethodGet, "size=257", http.StatusBadRequest, -1, ""},
		{"head above cap", http.MethodHead, "size=257", http.StatusBadRequest, -1, ""},
		{"status cannot bypass cap", http.MethodGet, "status=204&size=257", http.StatusBadRequest, -1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{MaxRequestSize: 64 << 10, MaxResponseSize: 256, ResponseControls: true}
			recorder := httptest.NewRecorder()
			HTTPHandler(cfg, "HTTP")(recorder, httptest.NewRequest(tc.method, "/?"+tc.query, nil))
			assert.Equal(t, tc.status, recorder.Code)
			assert.Equal(t, tc.contentLength, recorder.Header().Get("Content-Length"))
			if tc.bodyBytes >= 0 {
				assert.Equal(t, tc.bodyBytes, recorder.Body.Len())
			} else {
				assert.Contains(t, recorder.Body.String(), "size must be between 0 and 256 bytes")
			}
		})
	}
}

func TestConfiguredResponseCapRejectsOversizedReflectedJSON(t *testing.T) {
	cfg := &config.Config{MaxRequestSize: 64 << 10, MaxResponseSize: 256, PrintHeaders: true}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Sample", strings.Repeat("<&>", 256))
	tooLarge := metrics.ErrorsTotal.WithLabelValues("HTTP", "response_too_large")
	before := testutil.ToFloat64(tooLarge)
	recorder := httptest.NewRecorder()
	HTTPHandler(cfg, "HTTP")(recorder, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	assert.Equal(t, "Echo response too large\n", recorder.Body.String())
	assert.LessOrEqual(t, int64(recorder.Body.Len()), cfg.MaxResponseSize)
	assert.Equal(t, before+1, testutil.ToFloat64(tooLarge))
}

func TestConfiguredResponseCapCountsJSONEscaping(t *testing.T) {
	cfg := &config.Config{MaxRequestSize: 64 << 10, PrintHeaders: true}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	baseline, err := json.Marshal(NewHTTPResponse(req, cfg, "HTTP"))
	require.NoError(t, err)
	header := strings.Repeat("<&", 32)
	// The baseline and unescaped header fit; encoding six bytes per '<'/'&' does not.
	cfg.MaxResponseSize = int64(len(baseline) + len(header) + 64)
	recorder := httptest.NewRecorder()
	HTTPHandler(cfg, "HTTP")(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, json.Valid(recorder.Body.Bytes()))
	require.LessOrEqual(t, int64(recorder.Body.Len()), cfg.MaxResponseSize)

	req.Header.Set("X-Sample", header)
	encoded, err := json.Marshal(NewHTTPResponse(req, cfg, "HTTP"))
	require.NoError(t, err)
	require.Greater(t, int64(len(encoded)), cfg.MaxResponseSize)
	recorder = httptest.NewRecorder()
	HTTPHandler(cfg, "HTTP")(recorder, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), header)
}
