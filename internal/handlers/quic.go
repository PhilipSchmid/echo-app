package handlers

import (
	"net/http"

	"github.com/PhilipSchmid/echo-app/internal/config"
)

// QUICHandler returns an HTTP handler for QUIC
func QUICHandler(cfg *config.Config) http.HandlerFunc {
	return HTTPHandler(cfg, "QUIC") // Pass "QUIC" as the listener type
}
