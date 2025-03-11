package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PhilipSchmid/echo-app/internal/config"
)

func TestHTTPHandler(t *testing.T) {
	cfg := &config.Config{
		Message:      "Test Message",
		Node:         "Test Node",
		PrintHeaders: true,
	}

	handler := HTTPHandler(cfg, "HTTP")
	req := httptest.NewRequest("GET", "http://localhost:8080/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var response HTTPResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	if response.Message != "Test Message" {
		t.Errorf("Expected message 'Test Message', got '%s'", response.Message)
	}
	if response.Node != "Test Node" {
		t.Errorf("Expected node 'Test Node', got '%s'", response.Node)
	}
	if response.Listener != "HTTP" {
		t.Errorf("Expected listener 'HTTP', got '%s'", response.Listener)
	}
}
