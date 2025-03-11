package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PhilipSchmid/echo-app/internal/config"
)

func TestQUICHandler(t *testing.T) {
	cfg := &config.Config{
		Message:      "Test QUIC",
		Node:         "Test Node",
		PrintHeaders: true,
	}

	handler := QUICHandler(cfg)
	req := httptest.NewRequest("GET", "http://localhost:4433/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}
