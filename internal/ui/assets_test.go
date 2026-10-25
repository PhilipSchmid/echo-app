package ui

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticAssetCompression(t *testing.T) {
	handler := NewHandler(&config.Config{}, "HTTP")
	for _, path := range []string{"/ui/app.css", "/ui/app.js", "/ui/logo.svg"} {
		t.Run(path, func(t *testing.T) {
			identity := httptest.NewRecorder()
			handler.ServeHTTP(identity, httptest.NewRequest(http.MethodGet, path, nil))
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("Accept-Encoding", "gzip")
			compressed := httptest.NewRecorder()
			handler.ServeHTTP(compressed, request)
			require.Equal(t, http.StatusOK, compressed.Code)
			assert.Equal(t, "gzip", compressed.Header().Get("Content-Encoding"))
			assert.Equal(t, "Accept-Encoding", compressed.Header().Get("Vary"))
			assert.Equal(t, strconv.Itoa(compressed.Body.Len()), compressed.Header().Get("Content-Length"))
			assert.NotEqual(t, identity.Header().Get("ETag"), compressed.Header().Get("ETag"))
			assert.Less(t, compressed.Body.Len(), identity.Body.Len())
			reader, err := gzip.NewReader(bytes.NewReader(compressed.Body.Bytes()))
			require.NoError(t, err)
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.NoError(t, reader.Close())
			assert.Equal(t, identity.Body.Bytes(), data)
			t.Logf("identity=%d gzip=%d bytes (%.1f%% smaller)", identity.Body.Len(), compressed.Body.Len(), 100*(1-float64(compressed.Body.Len())/float64(identity.Body.Len())))
			head := httptest.NewRecorder()
			request.Method = http.MethodHead
			handler.ServeHTTP(head, request)
			assert.Equal(t, http.StatusOK, head.Code)
			assert.Empty(t, head.Body.Bytes())
			assert.Equal(t, compressed.Header(), head.Header())
		})
	}
	font := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ui/nunito-latin.woff2", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	handler.ServeHTTP(font, request)
	assert.Empty(t, font.Header().Get("Content-Encoding"))
	assert.Empty(t, font.Header().Get("Vary"))
	assert.NotEmpty(t, font.Header().Get("ETag"))
}

func TestStaticAssetEncodingNegotiation(t *testing.T) {
	handler := NewHandler(&config.Config{}, "HTTP")
	for _, test := range []struct {
		encoding, want string
		status         int
	}{
		{"", "", http.StatusOK},
		{"gzip", "gzip", http.StatusOK},
		{"br, GZip; q=1", "gzip", http.StatusOK},
		{"gzip;q=0", "", http.StatusOK},
		{"gzip;q=0, *;q=1", "", http.StatusOK},
		{"gzip;q=0.5, identity;q=0.1", "gzip", http.StatusOK},
		{"gzip;q=0.1, identity;q=1", "", http.StatusOK},
		{"*", "gzip", http.StatusOK},
		{"*;q=0, gzip;q=1", "gzip", http.StatusOK},
		{"*;q=0, identity;q=1", "", http.StatusOK},
		{"gzip;q=invalid", "", http.StatusOK},
		{"gzip;q=NaN", "", http.StatusOK},
		{"gzip;q=2", "", http.StatusOK},
		{"xgzip", "", http.StatusOK},
		{"gzip;q=0, identity;q=0", "", http.StatusNotAcceptable},
		{"*;q=0", "", http.StatusNotAcceptable},
	} {
		t.Run(test.encoding, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				request := httptest.NewRequest(method, "/ui/app.css", nil)
				request.Header.Set("Accept-Encoding", test.encoding)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				assert.Equal(t, test.status, response.Code)
				assert.Equal(t, test.want, response.Header().Get("Content-Encoding"))
				if method == http.MethodHead {
					assert.Empty(t, response.Body.Bytes())
				}
			}
		})
	}
	request := httptest.NewRequest(http.MethodGet, "/ui/app.css", nil)
	request.Header.Add("Accept-Encoding", "br")
	request.Header.Add("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
}

func TestStaticAssetRevalidation(t *testing.T) {
	handler := NewHandler(&config.Config{}, "HTTP")
	for _, encoding := range []string{"", "gzip"} {
		request := httptest.NewRequest(http.MethodGet, "/ui/app.css", nil)
		request.Header.Set("Accept-Encoding", encoding)
		initial := httptest.NewRecorder()
		handler.ServeHTTP(initial, request)
		etag := initial.Header().Get("ETag")
		require.NotEmpty(t, etag)
		for _, condition := range []string{etag, "W/" + etag, `"unrelated", ` + etag, "*"} {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				request.Method = method
				request.Header.Set("If-None-Match", condition)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				assert.Equal(t, http.StatusNotModified, response.Code)
				assert.Empty(t, response.Body.Bytes())
				assert.Empty(t, response.Header().Get("Content-Length"))
				assert.Equal(t, etag, response.Header().Get("ETag"))
				assert.Equal(t, initial.Header().Get("Vary"), response.Header().Get("Vary"))
				assert.Equal(t, initial.Header().Get("Cache-Control"), response.Header().Get("Cache-Control"))
			}
		}
		request.Method = http.MethodGet
		request.Header.Set("If-None-Match", `"unrelated"`)
		request.Header.Set("Range", "bytes=0-1,3-4,6-7")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, initial.Body.Bytes(), response.Body.Bytes())
		assert.Empty(t, response.Header().Get("Content-Range"))
		assert.Empty(t, response.Header().Get("Accept-Ranges"))
	}
}

func TestStaticAssetPreservesOtherCacheVariants(t *testing.T) {
	response := httptest.NewRecorder()
	response.Header().Add("Vary", "Origin")
	request := httptest.NewRequest(http.MethodGet, "/ui/app.css", nil)
	NewHandler(&config.Config{}, "HTTP").ServeHTTP(response, request)
	assert.Equal(t, []string{"Origin", "Accept-Encoding"}, response.Header().Values("Vary"))
}

func TestPrivateUIResponsesNeverRevalidateOrCompress(t *testing.T) {
	handler := NewHandler(&config.Config{PrintHeaders: true}, "HTTP")
	for _, path := range []string{"/ui/", "/ui/workbench/", "/ui/connect/", "/ui/examples/", "/ui/privacy/", "/ui/config.json", "/ui/certificate.json"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("If-None-Match", "*")
			request.Header.Set("Accept-Encoding", "gzip")
			request.Header.Set("Cookie", "secret=value")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, http.StatusOK, response.Code)
			assert.NotEmpty(t, response.Body.Bytes())
			assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			assert.Empty(t, response.Header().Get("ETag"))
			assert.Empty(t, response.Header().Get("Content-Encoding"))
			assert.NotContains(t, response.Body.String(), "secret=value")
		})
	}
}

func TestHomeResponseDoesNotReuseAnotherVisitorsRequest(t *testing.T) {
	handler := NewHandler(&config.Config{PrintHeaders: true}, "HTTP")
	for _, marker := range []string{"first-visitor", "second-visitor"} {
		request := httptest.NewRequest(http.MethodGet, "/ui/", nil)
		request.Header.Set("X-Visitor", marker)
		request.Header.Set("If-None-Match", "*")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), marker)
		if marker == "second-visitor" {
			assert.NotContains(t, response.Body.String(), "first-visitor")
		}
	}
}
