package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPObservationIncludesFailedAndCanceledRequests(t *testing.T) {
	for _, tc := range []struct {
		name, outcome string
		cancel, panic bool
		status        int
	}{
		{name: "implicit success", outcome: "200"},
		{name: "bad request", outcome: "400", status: 400},
		{name: "body too large", outcome: "413", status: 413},
		{name: "internal failure", outcome: "500", status: 500},
		{name: "canceled before response", outcome: "canceled", cancel: true},
		{name: "panic before response", outcome: "panic", panic: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := RequestsTotal.WithLabelValues("HTTP", "GET", "other")
			responses := HTTPResponsesTotal.WithLabelValues("HTTP", "other", tc.outcome)
			inFlight := InFlightRequests.WithLabelValues("HTTP")
			beforeRequests, beforeResponses, beforeInFlight := testutil.ToFloat64(requests), testutil.ToFloat64(responses), testutil.ToFloat64(inFlight)
			handler := InstrumentHTTP("HTTP", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				assert.Equal(t, beforeInFlight+1, testutil.ToFloat64(inFlight))
				if tc.panic {
					panic("test panic")
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
			}))
			request := httptest.NewRequest(http.MethodGet, "/unbounded/path?secret=never-a-label", nil)
			if tc.cancel {
				ctx, cancel := context.WithCancel(request.Context())
				cancel()
				request = request.WithContext(ctx)
			}
			run := func() { handler.ServeHTTP(httptest.NewRecorder(), request) }
			if tc.panic {
				require.Panics(t, run, "observability must not swallow handler panics")
			} else {
				run()
			}
			assert.Equal(t, beforeRequests+1, testutil.ToFloat64(requests))
			assert.Equal(t, beforeResponses+1, testutil.ToFloat64(responses))
			assert.Equal(t, beforeInFlight, testutil.ToFloat64(inFlight))
		})
	}
}

func TestHTTPFinalStatusAndBodyBytes(t *testing.T) {
	for _, interim := range []int{http.StatusContinue, http.StatusEarlyHints} {
		t.Run(http.StatusText(interim), func(t *testing.T) {
			responses := HTTPResponsesTotal.WithLabelValues("TLS", "/", "201")
			bytes := HTTPResponseBytesTotal.WithLabelValues("TLS")
			beforeResponses, beforeBytes := testutil.ToFloat64(responses), testutil.ToFloat64(bytes)
			handler := InstrumentHTTP("TLS", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(interim)
				w.WriteHeader(http.StatusCreated)
				w.WriteHeader(http.StatusAccepted)
				_, err := w.Write([]byte("body"))
				assert.NoError(t, err)
			}))
			server := httptest.NewServer(handler)
			t.Cleanup(server.Close)
			response, err := http.Get(server.URL)
			require.NoError(t, err)
			defer func() { _ = response.Body.Close() }()
			data, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			assert.Equal(t, http.StatusCreated, response.StatusCode)
			assert.Equal(t, "body", string(data))
			require.Eventually(t, func() bool {
				return testutil.ToFloat64(responses) == beforeResponses+1 && testutil.ToFloat64(bytes) == beforeBytes+4
			}, time.Second, time.Millisecond)
		})
	}
}

type deadlineResponse struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (w *deadlineResponse) SetReadDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func TestHTTPObservationPreservesControllerAndOptionalInterfaces(t *testing.T) {
	w := &deadlineResponse{ResponseRecorder: httptest.NewRecorder()}
	deadline := time.Now().Add(time.Second)
	handler := InstrumentHTTP("QUIC", http.HandlerFunc(func(observed http.ResponseWriter, _ *http.Request) {
		_, flush := observed.(http.Flusher)
		_, hijack := observed.(http.Hijacker)
		_, push := observed.(http.Pusher)
		assert.True(t, flush)
		assert.False(t, hijack)
		assert.False(t, push)
		require.NoError(t, http.NewResponseController(observed).SetReadDeadline(deadline))
		require.NoError(t, http.NewResponseController(observed).Flush())
	}))
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, deadline, w.deadline)
	assert.True(t, w.Flushed)
}

func TestRequestLabelsBoundUnknownInputsAndGroupAssets(t *testing.T) {
	RequestsTotal.Reset()
	RequestDuration.Reset()
	t.Cleanup(func() { RequestsTotal.Reset(); RequestDuration.Reset() })
	registry := prometheus.NewRegistry()
	registry.MustRegister(RequestsTotal, RequestDuration)
	for i := range 1000 {
		value := time.Unix(int64(i), 0).String()
		RecordRequest(value, value, value, 0.01)
		RecordRequest("HTTP", "GET", "/ui/"+value+".js", 0.01)
	}
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		require.Len(t, family.Metric, 2)
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				assert.Contains(t, []string{"HTTP", "GET", "other", "ui_assets"}, label.GetValue())
			}
		}
	}
}

func TestHTTPProtocolAndIPLabelsUseObservedConnection(t *testing.T) {
	for _, tc := range []struct {
		major, minor int
		remote       string
		protocol, ip string
	}{
		{1, 0, "192.0.2.1:1234", "HTTP/1.0", "IPv4"},
		{1, 1, "[::ffff:192.0.2.1]:1234", "HTTP/1.1", "IPv4"},
		{2, 0, "[2001:db8::1]:1234", "HTTP/2", "IPv6"},
		{3, 0, "[::1]:1234", "HTTP/3", "IPv6"},
		{42, 0, "not-an-address", "other", "unknown"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.ProtoMajor, request.ProtoMinor, request.RemoteAddr = tc.major, tc.minor, tc.remote
		request.Header.Set("X-Forwarded-For", "2001:db8::99")
		protocol, ip := httpConnectionLabels(request)
		assert.Equal(t, tc.protocol, protocol)
		assert.Equal(t, tc.ip, ip)
	}
}

func TestHTTPProtocolMetricDescribesClientIPSource(t *testing.T) {
	HTTPProtocolRequestsTotal.Reset()
	t.Cleanup(HTTPProtocolRequestsTotal.Reset)
	registry := prometheus.NewRegistry()
	registry.MustRegister(HTTPProtocolRequestsTotal)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	// net/http sets RemoteAddr to the validated original client in trusted PROXY mode.
	request.RemoteAddr = "[2001:db8::1]:1234"
	request.ProtoMajor = 2
	request.Header.Set("X-Forwarded-For", "192.0.2.1")
	InstrumentHTTP("TLS", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(httptest.NewRecorder(), request)
	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)
	family := families[0]
	assert.Contains(t, family.GetHelp(), "client IP version")
	assert.Contains(t, family.GetHelp(), "trusted PROXY")
	require.Len(t, family.Metric, 1)
	labels := make(map[string]string)
	for _, label := range family.Metric[0].Label {
		labels[label.GetName()] = label.GetValue()
	}
	assert.Equal(t, map[string]string{"listener": "TLS", "protocol": "HTTP/2", "ip_version": "IPv6"}, labels)
	assert.Equal(t, float64(1), family.Metric[0].GetCounter().GetValue())
}
