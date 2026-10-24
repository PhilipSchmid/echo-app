package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/PhilipSchmid/echo-app/internal/config"
	"github.com/PhilipSchmid/echo-app/internal/metrics"
	"github.com/sirupsen/logrus"
)

// HTTPResponse defines the structure of the HTTP echo response
type HTTPResponse struct {
	BaseResponse
	HTTPVersion  string              `json:"http_version,omitempty"`
	HTTPMethod   string              `json:"http_method,omitempty"`
	HTTPEndpoint string              `json:"http_endpoint,omitempty"`
	Headers      map[string][]string `json:"headers,omitempty"`
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

		response := buildHTTPResponse(r, cfg, listener)
		data, err := json.Marshal(response)
		if err != nil {
			logrus.Errorf("Failed to marshal JSON: %v", err)
			metrics.RecordError(listener, "marshal_error")
			w.WriteHeader(http.StatusInternalServerError)
			if _, writeErr := w.Write([]byte("Internal Server Error")); writeErr != nil {
				logrus.Errorf("Failed to write error response: %v", writeErr)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, writeErr := w.Write(data); writeErr != nil {
			logrus.Errorf("Failed to write response: %v", writeErr)
			metrics.RecordError(listener, "write_error")
		}
		duration := time.Since(start).Seconds()
		// Normalize endpoint to prevent high cardinality in metrics
		normalizedPath := normalizeEndpoint(r.URL.Path)
		metrics.RecordRequest(listener, r.Method, normalizedPath, duration)

		// Debug logging for response
		logrus.Debugf("[%s] Response sent: %d bytes in %.3fms", listener, len(data), duration*1000)
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
		BaseResponse: NewBaseResponse(cfg, effectiveListener, r.RemoteAddr),
		HTTPVersion:  r.Proto,
		HTTPMethod:   r.Method,
		HTTPEndpoint: r.URL.Path,
	}
	if cfg.PrintHeaders {
		response.Headers = diagnosticHeaders(r.Header, cfg.UnsafeHeaders)
	}
	return response
}
