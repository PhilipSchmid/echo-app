package handlers

import (
	"encoding/json"
	"net"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/metrics"
	"github.com/sirupsen/logrus"
)

// TCPResponse represents the expected structure of the TCP response
type TCPResponse struct {
	BaseResponse
}

func TCPHandler(conn net.Conn, cfg *config.Config) {
	start := time.Now()
	remoteAddr := conn.RemoteAddr().String()
	sourceIP := extractIP(remoteAddr)

	// Panic recovery to prevent handler crashes
	defer func() {
		if rec := recover(); rec != nil {
			logrus.Errorf("[TCP] Recovered from panic: %v", rec)
			metrics.RecordError("TCP", "panic")
		}
	}()

	// Enhanced request logging at INFO level for troubleshooting
	logrus.Infof("[TCP] Connection from %s", sourceIP)

	// Debug logging (keep existing for detailed debugging)
	logrus.Debugf("[TCP] New connection from %s", remoteAddr)

	defer func() {
		if err := conn.Close(); err != nil {
			logrus.Errorf("Failed to close TCP connection: %v", err)
		}
		duration := time.Since(start).Seconds()
		logrus.Debugf("[TCP] Connection closed from %s after %.3fms", remoteAddr, duration*1000)
	}()

	// Track connection
	metrics.ConnectionOpened("TCP")
	defer metrics.ConnectionClosed("TCP")

	defer func() {
		duration := time.Since(start).Seconds()
		metrics.RecordRequest("TCP", "connection", "", duration)
	}()

	response := buildTCPResponse(conn, cfg)
	data, err := json.Marshal(response)
	if err != nil {
		logrus.Errorf("Failed to marshal JSON: %v", err)
		metrics.RecordError("TCP", "marshal_error")
		return
	}
	if _, err := conn.Write(data); err != nil {
		logrus.Errorf("Failed to write to connection: %v", err)
		metrics.RecordError("TCP", "write_error")
	} else {
		logrus.Debugf("[TCP] Response sent to %s: %d bytes", remoteAddr, len(data))
	}
}

// buildTCPResponse constructs the response for TCP
func buildTCPResponse(conn net.Conn, cfg *config.Config) TCPResponse {
	return TCPResponse{
		BaseResponse: NewBaseResponse(cfg, "TCP", conn.RemoteAddr().String()),
	}
}
