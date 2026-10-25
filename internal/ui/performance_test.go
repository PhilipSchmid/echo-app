package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
)

// Discard the body as a socket would, so the benchmark measures rendering and
// asset preparation rather than a test recorder's extra body copy.
type benchmarkResponseWriter struct {
	header http.Header
}

func (w benchmarkResponseWriter) Header() http.Header { return w.header }
func (benchmarkResponseWriter) WriteHeader(int)       {}
func (benchmarkResponseWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func BenchmarkUI(b *testing.B) {
	handler := NewHandler(&config.Config{PrintHeaders: true, PublicURL: "https://echo.container.education"}, "HTTP")
	for _, test := range []struct {
		name, path, encoding string
	}{
		{"CSS", "/ui/app.css", ""},
		{"CSSGzip", "/ui/app.css", "gzip"},
		{"JavaScript", "/ui/app.js", ""},
		{"Logo", "/ui/logo.svg", ""},
		{"Font", "/ui/nunito-latin.woff2", ""},
		{"Workbench", "/ui/workbench/", ""},
		{"Home", "/ui/", ""},
	} {
		b.Run(test.name, func(b *testing.B) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.Header.Set("Accept-Encoding", test.encoding)
			request.Header.Set("User-Agent", "Echo Observatory benchmark")
			request.Header.Set("X-Request-ID", "sample-request")
			writer := benchmarkResponseWriter{header: make(http.Header)}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				clear(writer.header)
				handler.ServeHTTP(writer, request)
			}
		})
	}
}
