package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestHTTPHandler_ErrorScenarios(t *testing.T) {
	tests := []struct {
		name           string
		cfg            *config.Config
		remoteAddr     string
		expectedStatus int
		checkResponse  func(t *testing.T, body string)
	}{
		{
			name: "malformed remote address",
			cfg: &config.Config{
				Message:      "test",
				Node:         "test-node",
				PrintHeaders: false,
			},
			remoteAddr:     "invalid-address",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body string) {
				// Should still return valid response with original address
				assert.Contains(t, body, "invalid-address")
			},
		},
		{
			name: "empty remote address",
			cfg: &config.Config{
				Message:      "test",
				Node:         "test-node",
				PrintHeaders: false,
			},
			remoteAddr:     "",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body string) {
				assert.Contains(t, body, `"source_ip":""`)
			},
		},
		{
			name: "with headers enabled",
			cfg: &config.Config{
				Message:      "test",
				Node:         "test-node",
				PrintHeaders: true,
			},
			remoteAddr:     "192.168.1.1:12345",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body string) {
				assert.Contains(t, body, `"headers":{`)
				assert.Contains(t, body, "User-Agent")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := HTTPHandler(tt.cfg, "HTTP")

			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			req.Header.Set("User-Agent", "test-agent")

			w := httptest.NewRecorder()
			handler(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			if tt.checkResponse != nil {
				tt.checkResponse(t, w.Body.String())
			}
		})
	}
}

func TestHTTPHandler_ConcurrentRequests(t *testing.T) {
	cfg := &config.Config{
		Message:      "concurrent-test",
		Node:         "test-node",
		PrintHeaders: false,
	}

	handler := HTTPHandler(cfg, "HTTP")

	// Run multiple concurrent requests
	numRequests := 100
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			req := httptest.NewRequest("GET", "/concurrent", nil)
			req.RemoteAddr = "192.168.1.1:12345"

			w := httptest.NewRecorder()
			handler(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}
}
