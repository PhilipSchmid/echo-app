package handlers

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/connection"
	"github.com/PhilipSchmid/echo-observatory/v2/internal/metrics"
	"github.com/sirupsen/logrus"
)

// HTTPResponse defines the structure of the HTTP echo response
type HTTPResponse struct {
	BaseResponse
	HTTPVersion           string              `json:"http_version,omitempty"`
	HTTPMethod            string              `json:"http_method,omitempty"`
	HTTPEndpoint          string              `json:"http_endpoint,omitempty"`
	HTTPHost              string              `json:"http_host,omitempty"`
	HTTPRequestURI        string              `json:"http_request_uri,omitempty"`
	RemoteAddr            string              `json:"remote_addr,omitempty"`
	LocalAddr             string              `json:"local_addr,omitempty"`
	LocalIPVersion        string              `json:"local_ip_version,omitempty"`
	SocketLocalAddr       string              `json:"socket_local_addr,omitempty"`
	SocketLocalIPVersion  string              `json:"socket_local_ip_version,omitempty"`
	TLSVersion            string              `json:"tls_version,omitempty"`
	TLSCipherSuite        string              `json:"tls_cipher_suite,omitempty"`
	TLSServerName         string              `json:"tls_server_name,omitempty"`
	TLSNegotiatedProtocol string              `json:"tls_negotiated_protocol,omitempty"`
	Headers               map[string][]string `json:"headers,omitempty"`
}

// HTTPHandler returns an HTTP handler function
func HTTPHandler(cfg *config.Config, listener string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		debug := logrus.IsLevelEnabled(logrus.DebugLevel)
		var start time.Time
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
		if cfg.UI && r.URL.Path == "/" {
			w.Header().Add("Link", `<ui/>; rel="alternate"; type="text/html"; title="Echo Observatory"`)
		}

		// Panic recovery to prevent handler crashes
		defer func() {
			if rec := recover(); rec != nil {
				logrus.Errorf("[%s] Recovered from panic", listener)
				metrics.RecordError(listener, "panic")
				w.WriteHeader(http.StatusInternalServerError)
				if _, writeErr := w.Write([]byte("Internal Server Error")); writeErr != nil {
					logrus.Debug("Failed to write panic response")
				}
			}
		}()

		// Request diagnostics are opt-in; skip formatting and metadata work otherwise.
		if debug {
			start = time.Now()
			logrus.Debugf("[%s] Request: %s %s from %s", listener, r.Method, r.URL.Path, r.RemoteAddr)
		}

		// Read through the limit so oversized bodies are rejected without retaining them.
		if r.Body != nil {
			body := http.MaxBytesReader(w, r.Body, cfg.MaxRequestSize)
			defer func() { _ = body.Close() }()
			if r.ContentLength > cfg.MaxRequestSize {
				metrics.RecordError(listener, "request_too_large")
				http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			if _, err := io.Copy(io.Discard, body); err != nil {
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					metrics.RecordError(listener, "request_too_large")
					http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
				} else {
					metrics.RecordError(listener, "read_error")
					http.Error(w, "Failed to read request body", http.StatusBadRequest)
				}
				return
			}
		}

		if debug && cfg.PrintHeaders {
			logrus.Debugf("[%s] Headers: Host=%s, Content-Type=%s, Accept=%s",
				listener,
				r.Host,
				r.Header.Get("Content-Type"),
				r.Header.Get("Accept"))
			logrus.Debugf("[%s] Request headers: %+v", listener, diagnosticHeaders(r.Header, false))
		}

		controls := responseControls{status: http.StatusOK, size: -1}
		if cfg.ResponseControls {
			var err error
			controls, err = parseResponseControls(r.URL.RawQuery, responseLimit(cfg))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if controls.delay > 0 {
				timer := time.NewTimer(controls.delay)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-r.Context().Done():
					return
				}
			}
		}

		allowsBody := controls.status != http.StatusNoContent && controls.status != http.StatusResetContent && controls.status != http.StatusNotModified
		var data []byte
		w.Header().Set("Content-Type", "application/json")
		if controls.size >= 0 {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		if allowsBody {
			if controls.size >= 0 {
				w.Header().Set("Content-Length", strconv.Itoa(controls.size))
			} else {
				var err error
				data, err = json.Marshal(NewHTTPResponse(r, cfg, listener))
				if err != nil {
					metrics.RecordError(listener, "marshal_error")
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				if int64(len(data)) > responseLimit(cfg) {
					metrics.RecordError(listener, "response_too_large")
					http.Error(w, "Echo response too large", http.StatusRequestEntityTooLarge)
					return
				}
				w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			}
		}
		w.WriteHeader(controls.status)
		var written int64
		if allowsBody && r.Method != http.MethodHead {
			var writeErr error
			if controls.size >= 0 {
				written, writeErr = io.CopyN(w, zeroReader{}, int64(controls.size))
			} else {
				var n int
				n, writeErr = w.Write(data)
				written = int64(n)
			}
			if writeErr != nil {
				logrus.Debug("Failed to write response")
				metrics.RecordError(listener, "write_error")
			}
		}
		if debug {
			logrus.Debugf("[%s] Response sent: %d bytes in %.3fms", listener, written, float64(time.Since(start))/float64(time.Millisecond))
		}
	}
}

// NewHTTPResponse describes the request received by an HTTP listener.
func NewHTTPResponse(r *http.Request, cfg *config.Config, listener string) HTTPResponse {
	// When running in H2C mode the listener serves both HTTP/1.1 and HTTP/2
	// cleartext on the same port. Reflect the actually negotiated protocol so
	// the response is meaningful rather than always showing "H2C".
	effectiveListener := listener
	if listener == "H2C" && r.ProtoMajor < 2 {
		effectiveListener = "HTTP"
	}
	response := HTTPResponse{
		BaseResponse:   NewBaseResponse(cfg, effectiveListener, r.RemoteAddr),
		HTTPVersion:    r.Proto,
		HTTPMethod:     r.Method,
		HTTPEndpoint:   r.URL.Path,
		HTTPHost:       r.Host,
		HTTPRequestURI: r.RequestURI,
		RemoteAddr:     r.RemoteAddr,
	}
	setSocketPeer(&response.BaseResponse, connection.RemoteAddr(r.Context()))
	if response.HTTPRequestURI == "" {
		response.HTTPRequestURI = r.URL.RequestURI()
	}
	if local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		response.LocalAddr = local.String()
		response.LocalIPVersion = ipVersion(extractIP(local.String()))
		if socket := connection.SocketAddr(local); socket != nil {
			response.SocketLocalAddr = socket.String()
			response.SocketLocalIPVersion = ipVersion(extractIP(socket.String()))
		}
	}
	if r.TLS != nil {
		response.TLSVersion = tls.VersionName(r.TLS.Version)
		response.TLSCipherSuite = tls.CipherSuiteName(r.TLS.CipherSuite)
		response.TLSServerName = r.TLS.ServerName
		response.TLSNegotiatedProtocol = r.TLS.NegotiatedProtocol
	}
	if cfg.PrintHeaders {
		response.Headers = diagnosticHeaders(r.Header, cfg.UnsafeHeaders)
	}
	return response
}
