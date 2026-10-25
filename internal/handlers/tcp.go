package handlers

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/sirupsen/logrus"
)

// TCPResponse represents the expected structure of the TCP response
type TCPResponse struct {
	BaseResponse
}

func TCPHandler(ctx context.Context, conn net.Conn, cfg *config.Config) {
	start := time.Now()
	remoteAddr := conn.RemoteAddr().String()
	debug := logrus.IsLevelEnabled(logrus.DebugLevel)

	// Panic recovery to prevent handler crashes
	defer func() {
		if rec := recover(); rec != nil {
			logrus.Error("[TCP] Recovered from panic")
			metrics.RecordError("TCP", "panic")
		}
	}()

	defer func() {
		if err := conn.Close(); err != nil {
			logrus.Debug("Failed to close TCP connection")
		}
		if debug {
			logrus.Debugf("[TCP] Connection closed from %s after %.3fms", remoteAddr, float64(time.Since(start))/float64(time.Millisecond))
		}
	}()

	// Track connection
	metrics.ConnectionOpened("TCP")
	defer metrics.ConnectionClosed("TCP")

	defer func() {
		duration := time.Since(start).Seconds()
		metrics.RecordRequest("TCP", "connection", "", duration)
	}()

	// Accepted connections still need cleanup and accounting during shutdown.
	if ctx.Err() != nil {
		if debug {
			logrus.Debugf("[TCP] Context cancelled before processing connection from %s", remoteAddr)
		}
		return
	}

	if debug {
		logrus.Debugf("[TCP] Connection from %s", remoteAddr)
	}

	response := TCPResponse{BaseResponse: NewBaseResponse(cfg, "TCP", remoteAddr)}
	setSocketPeer(&response.BaseResponse, conn.RemoteAddr())
	data, err := json.Marshal(response)
	if err != nil {
		logrus.Error("Failed to marshal TCP response")
		metrics.RecordError("TCP", "marshal_error")
		return
	}
	limit := cfg.MaxResponseSize
	if limit <= 0 {
		limit = maxResponseSize
	}
	if int64(len(data)) > limit {
		metrics.RecordError("TCP", "response_too_large")
		return
	}
	if _, err := conn.Write(data); err != nil {
		if debug {
			logrus.Debug("Failed to write to TCP connection")
		}
		metrics.RecordError("TCP", "write_error")
	} else if debug {
		logrus.Debugf("[TCP] Response sent to %s: %d bytes", remoteAddr, len(data))
	}
}
