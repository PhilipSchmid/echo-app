package metrics

import (
	"net/http"
	"net/netip"
	"strconv"
	"time"
)

// InstrumentHTTP observes the entire listener mux, including rejected requests.
func InstrumentHTTP(listener string, next http.Handler) http.Handler {
	listener = listenerLabel(listener)
	inFlight := InFlightRequests.WithLabelValues(listener)
	bytes := HTTPResponseBytesTotal.WithLabelValues(listener)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		observed := &responseObserver{ResponseWriter: w}
		panicked := true
		inFlight.Inc()
		defer func() {
			inFlight.Dec()
			endpoint := endpointLabel(r.URL.Path)
			RecordRequest(listener, r.Method, endpoint, time.Since(start).Seconds())
			outcome := strconv.Itoa(observed.status)
			if observed.status < 100 || observed.status > 599 {
				outcome = "other"
			}
			if panicked {
				RecordError(listener, "panic")
			}
			if observed.status == 0 {
				switch {
				case panicked:
					outcome = "panic"
				case r.Context().Err() != nil:
					outcome = "canceled"
					RecordError(listener, "canceled")
				default:
					outcome = "200"
				}
			}
			HTTPResponsesTotal.WithLabelValues(listener, endpoint, outcome).Inc()
			protocol, ipVersion := httpConnectionLabels(r)
			HTTPProtocolRequestsTotal.WithLabelValues(listener, protocol, ipVersion).Inc()
			bytes.Add(float64(observed.bytes))
		}()
		next.ServeHTTP(observed.writer(), r)
		panicked = false
	})
}

func httpConnectionLabels(r *http.Request) (protocol, ipVersion string) {
	protocol = "other"
	switch {
	case r.ProtoMajor == 1 && r.ProtoMinor == 0:
		protocol = "HTTP/1.0"
	case r.ProtoMajor == 1 && r.ProtoMinor == 1:
		protocol = "HTTP/1.1"
	case r.ProtoMajor == 2:
		protocol = "HTTP/2"
	case r.ProtoMajor == 3:
		protocol = "HTTP/3"
	}
	ipVersion = "unknown"
	addr, err := netip.ParseAddrPort(r.RemoteAddr)
	if err == nil {
		if addr.Addr().Unmap().Is4() {
			ipVersion = "IPv4"
		} else {
			ipVersion = "IPv6"
		}
	}
	return protocol, ipVersion
}

type responseObserver struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *responseObserver) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *responseObserver) WriteHeader(status int) {
	// Interim responses do not replace the eventual final response status.
	if w.status == 0 && (status >= 200 || status == http.StatusSwitchingProtocols) {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseObserver) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += int64(n)
	return n, err
}

func (w *responseObserver) FlushError() error {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

type observedFlusher struct{ *responseObserver }

func (w observedFlusher) Flush() { _ = w.FlushError() }

// Preserve the optional net/http interfaces without claiming unsupported ones.
func (w *responseObserver) writer() http.ResponseWriter {
	f, flush := w.ResponseWriter.(http.Flusher)
	h, hijack := w.ResponseWriter.(http.Hijacker)
	p, push := w.ResponseWriter.(http.Pusher)
	if flush {
		f = observedFlusher{w}
	}
	switch {
	case flush && hijack && push:
		return struct {
			*responseObserver
			http.Flusher
			http.Hijacker
			http.Pusher
		}{w, f, h, p}
	case flush && hijack:
		return struct {
			*responseObserver
			http.Flusher
			http.Hijacker
		}{w, f, h}
	case flush && push:
		return struct {
			*responseObserver
			http.Flusher
			http.Pusher
		}{w, f, p}
	case hijack && push:
		return struct {
			*responseObserver
			http.Hijacker
			http.Pusher
		}{w, h, p}
	case flush:
		return struct {
			*responseObserver
			http.Flusher
		}{w, f}
	case hijack:
		return struct {
			*responseObserver
			http.Hijacker
		}{w, h}
	case push:
		return struct {
			*responseObserver
			http.Pusher
		}{w, p}
	default:
		return w
	}
}
