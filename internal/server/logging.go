package server

import (
	"context"
	"log"
	"log/slog"
	"strings"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/grpclog"
)

// ConfigureProtocolLogging must run once at startup, before protocol goroutines.
func ConfigureProtocolLogging() {
	grpclog.SetLoggerV2(grpcDiagnostics{})
	log.SetFlags(0)
	log.SetPrefix("")
	log.SetOutput(protocolDiagnostics{listener: "Transport"})
}

type protocolDiagnostics struct{ listener string }

func (d protocolDiagnostics) Write(data []byte) (int, error) {
	entry := logrus.WithField("listener", d.listener)
	message := string(data)
	// Library text can contain peer addresses, headers, URLs or panic payloads.
	switch {
	case strings.HasPrefix(message, "http: panic serving "), strings.HasPrefix(message, "http2: panic serving "):
		entry.Error("HTTP handler panicked")
	case strings.HasPrefix(message, "http: Accept error: "):
		entry.Warn("HTTP listener temporarily unable to accept connections")
	case strings.HasPrefix(message, "http: TLS handshake error from "):
		entry.Debug("TLS handshake failed")
	case strings.HasPrefix(message, "http: invalid Content-Length of "):
		entry.Error("HTTP handler returned an invalid Content-Length")
	case strings.HasPrefix(message, "http: superfluous response.WriteHeader"),
		strings.HasPrefix(message, "http: response.WriteHeader on hijacked connection"),
		strings.HasPrefix(message, "http: response.Write on hijacked connection"),
		strings.HasPrefix(message, "http: WriteHeader called with both Transfer-Encoding"):
		entry.Warn("HTTP handler wrote inconsistent response headers")
	case strings.HasPrefix(message, "failed to sufficiently increase receive buffer size"):
		entry.Warn("QUIC receive buffer is smaller than requested")
	case strings.HasPrefix(message, "failed to sufficiently increase send buffer size"):
		entry.Warn("QUIC send buffer is smaller than requested")
	default:
		entry.Debug("Protocol transport diagnostic")
	}
	return len(data), nil
}

func httpErrorLog(listener string) *log.Logger {
	return log.New(protocolDiagnostics{listener: listener}, "", 0)
}

type http3Diagnostics struct{}

func (http3Diagnostics) Enabled(_ context.Context, level slog.Level) bool {
	if level >= slog.LevelError {
		return logrus.IsLevelEnabled(logrus.ErrorLevel)
	}
	if level >= slog.LevelWarn {
		return logrus.IsLevelEnabled(logrus.WarnLevel)
	}
	return logrus.IsLevelEnabled(logrus.DebugLevel)
}

func (http3Diagnostics) Handle(_ context.Context, record slog.Record) error {
	entry := logrus.WithField("listener", "QUIC")
	switch record.Message {
	case "handling request":
		return nil
	case "http3: panic serving":
		entry.Error("HTTP/3 handler panicked")
	case "Malformed Content-Length":
		entry.Error("HTTP/3 handler returned an invalid Content-Length")
	case "Unable to extract port from listener, will not be announced using SetQUICHeaders":
		entry.Warn("HTTP/3 port advertisement is unavailable")
	default:
		switch {
		case record.Level >= slog.LevelError:
			entry.Error("HTTP/3 internal diagnostic")
		case record.Level >= slog.LevelWarn:
			entry.Warn("HTTP/3 transport warning")
		default:
			entry.Debug("HTTP/3 transport diagnostic")
		}
	}
	return nil
}

func (h http3Diagnostics) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h http3Diagnostics) WithGroup(string) slog.Handler      { return h }

type grpcDiagnostics struct{}

func (grpcDiagnostics) Info(...any)             {}
func (grpcDiagnostics) Infoln(...any)           {}
func (grpcDiagnostics) Infof(string, ...any)    {}
func (grpcDiagnostics) V(int) bool              { return false }
func (grpcDiagnostics) Warning(...any)          { logrus.Debug("gRPC transport warning") }
func (grpcDiagnostics) Warningln(...any)        { logrus.Debug("gRPC transport warning") }
func (grpcDiagnostics) Warningf(string, ...any) { logrus.Debug("gRPC transport warning") }
func (grpcDiagnostics) Error(...any)            { logrus.Error("gRPC internal error") }
func (grpcDiagnostics) Errorln(...any)          { logrus.Error("gRPC internal error") }
func (grpcDiagnostics) Errorf(string, ...any)   { logrus.Error("gRPC internal error") }
func (grpcDiagnostics) Fatal(...any)            { logrus.Fatal("gRPC fatal error") }
func (grpcDiagnostics) Fatalln(...any)          { logrus.Fatal("gRPC fatal error") }
func (grpcDiagnostics) Fatalf(string, ...any)   { logrus.Fatal("gRPC fatal error") }
