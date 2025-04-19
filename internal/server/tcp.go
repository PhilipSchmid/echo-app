package server

import (
	"net"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/handlers"
	"github.com/sirupsen/logrus"
)

// StartTCPServer starts the TCP server
func StartTCPServer(cfg *config.Config) {
	listener, err := net.Listen("tcp", ":"+cfg.TCPPort)
	if err != nil {
		logrus.Fatalf("Failed to start TCP server: %v", err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			logrus.Errorf("Failed to close TCP listener: %v", err)
		}
	}()

	logrus.Infof("TCP server listening on port %s", cfg.TCPPort)
	for {
		conn, err := listener.Accept()
		if err != nil {
			logrus.Errorf("Failed to accept TCP connection: %v", err)
			continue
		}
		go handlers.TCPHandler(conn, cfg)
	}
}
