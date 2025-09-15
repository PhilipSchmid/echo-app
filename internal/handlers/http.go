package handlers

import (
	"encoding/json"
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

		// Enhanced request logging at INFO level for troubleshooting
		sourceIP := extractIP(r.RemoteAddr)
		userAgent := r.Header.Get("User-Agent")
		if userAgent == "" {
			userAgent = "unknown"
		}

		logrus.Infof("[%s] Request: %s %s from %s (User-Agent: %s)",
			listener, r.Method, r.URL.Path, sourceIP, userAgent)

		// Additional header information if configured
		if cfg.PrintHeaders {
			logrus.Infof("[%s] Headers: Host=%s, Content-Type=%s, Accept=%s",
				listener,
				r.Header.Get("Host"),
				r.Header.Get("Content-Type"),
				r.Header.Get("Accept"))
		}

		// Debug logging (keep existing for detailed debugging)
		logrus.Debugf("[%s] Incoming request: %s %s from %s", listener, r.Method, r.URL.Path, r.RemoteAddr)
		if logrus.GetLevel() >= logrus.DebugLevel && cfg.PrintHeaders {
			logrus.Debugf("[%s] Request headers: %+v", listener, r.Header)
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
		metrics.RecordRequest(listener, r.Method, r.URL.Path, duration)

		// Debug logging for response
		logrus.Debugf("[%s] Response sent: %d bytes in %.3fms", listener, len(data), duration*1000)
	}
}

// buildHTTPResponse constructs the response struct
func buildHTTPResponse(r *http.Request, cfg *config.Config, listener string) HTTPResponse {
	response := HTTPResponse{
		BaseResponse: NewBaseResponse(cfg, listener, r.RemoteAddr),
		HTTPVersion:  r.Proto,
		HTTPMethod:   r.Method,
		HTTPEndpoint: r.URL.Path,
	}
	if cfg.PrintHeaders {
		response.Headers = r.Header
	}
	return response
}
