package server

import (
	"crypto/tls"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/handlers"
	"github.com/PhilipSchmid/echo-app/internal/utils"
	"github.com/quic-go/quic-go/http3"
	"github.com/sirupsen/logrus"
)

// StartQUICServer starts the QUIC server
func StartQUICServer(cfg *config.Config) {
	cert, err := utils.GenerateSelfSignedCert()
	if err != nil {
		logrus.Fatalf("Failed to generate self-signed certificate: %v", err)
	}

	server := &http3.Server{
		Addr:    ":" + cfg.QUICPort,
		Handler: handlers.QUICHandler(cfg),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}

	logrus.Infof("QUIC server listening on port %s", cfg.QUICPort)
	if err := server.ListenAndServe(); err != nil {
		logrus.Errorf("QUIC server failed: %v", err)
	}
}
