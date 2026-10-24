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

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/metrics"
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
	TLSVersion            string              `json:"tls_version,omitempty"`
	TLSCipherSuite        string              `json:"tls_cipher_suite,omitempty"`
	TLSServerName         string              `json:"tls_server_name,omitempty"`
	TLSNegotiatedProtocol string              `json:"tls_negotiated_protocol,omitempty"`
	Headers               map[string][]string `json:"headers,omitempty"`
}

// HTTPHandler returns an HTTP handler function
func HTTPHandler(cfg *config.Config, listener string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set("Cache-Control", "no-store")

		// Panic recovery to prevent handler crashes
		defer func() {
			if rec := recover(); rec != nil {
				logrus.Errorf("[%s] Recovered from panic: %v", listener, rec)
				metrics.RecordError(listener, "panic")
				w.WriteHeader(http.StatusInternalServerError)
				if _, writeErr := w.Write([]byte("Internal Server Error")); writeErr != nil {
					logrus.Errorf("Failed to write panic response: %v", writeErr)
				}
			}
		}()

		if r.ProtoMajor == 3 {
			controller := http.NewResponseController(w)
			for _, setDeadline := range []func(time.Time) error{controller.SetReadDeadline, controller.SetWriteDeadline} {
				if err := setDeadline(start.Add(30 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
					metrics.RecordError(listener, "deadline_error")
					http.Error(w, "Failed to set stream deadline", http.StatusInternalServerError)
					return
				}
			}
		}

		// Enhanced request logging at INFO level for troubleshooting
		sourceIP := extractIP(r.RemoteAddr)
		userAgent := r.Header.Get("User-Agent")
		if userAgent == "" {
			userAgent = "unknown"
		}

		logrus.Infof("[%s] Request: %s %s from %s (User-Agent: %s)",
			listener, r.Method, r.URL.Path, sourceIP, userAgent)

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

		// Additional header information if configured
		if cfg.PrintHeaders {
			logrus.Infof("[%s] Headers: Host=%s, Content-Type=%s, Accept=%s",
				listener,
				r.Host,
				r.Header.Get("Content-Type"),
				r.Header.Get("Accept"))
		}

		// Debug logging (keep existing for detailed debugging)
		logrus.Debugf("[%s] Incoming request: %s %s from %s", listener, r.Method, r.URL.Path, r.RemoteAddr)
		if logrus.GetLevel() >= logrus.DebugLevel && cfg.PrintHeaders {
			logrus.Debugf("[%s] Request headers: %+v", listener, diagnosticHeaders(r.Header, cfg.UnsafeHeaders))
		}

		controls := responseControls{status: http.StatusOK, size: -1}
		if cfg.ResponseControls {
			var err error
			controls, err = parseResponseControls(r.URL.RawQuery)
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
				data, err = json.Marshal(buildHTTPResponse(r, cfg, listener))
				if err != nil {
					metrics.RecordError(listener, "marshal_error")
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
				logrus.Errorf("Failed to write response: %v", writeErr)
				metrics.RecordError(listener, "write_error")
			}
		}
		duration := time.Since(start).Seconds()
		// Normalize endpoint to prevent high cardinality in metrics
		normalizedPath := normalizeEndpoint(r.URL.Path)
		metrics.RecordRequest(listener, r.Method, normalizedPath, duration)

		// Debug logging for response
		logrus.Debugf("[%s] Response sent: %d bytes in %.3fms", listener, written, duration*1000)
	}
}

// buildHTTPResponse constructs the response struct
func buildHTTPResponse(r *http.Request, cfg *config.Config, listener string) HTTPResponse {
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
	if response.HTTPRequestURI == "" {
		response.HTTPRequestURI = r.URL.RequestURI()
	}
	if local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		response.LocalAddr = local.String()
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
