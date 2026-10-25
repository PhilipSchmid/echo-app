package server

import (
	"crypto/tls"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/handlers"
)

func listenerTLSConfig(cfg *config.Config, shared *tls.Config) (*tls.Config, error) {
	if shared != nil {
		return shared.Clone(), nil
	}
	return handlers.GetTLSConfig(cfg)
}
