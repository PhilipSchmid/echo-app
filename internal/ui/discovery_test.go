package ui

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func TestPublicDiscoveryDocumentsAndMetadata(t *testing.T) {
	for _, base := range []string{"https://echo.container.education/", "http://[::1]:18880/prefix/"} {
		handler := NewHandler(&config.Config{PublicURL: base, PrintHeaders: true}, "HTTP")
		for _, path := range []string{"/robots.txt", "/sitemap.xml", "/ui/", "/ui/workbench/", "/ui/examples/", "/ui/connect/", "/ui/privacy/"} {
			t.Run(base+path, func(t *testing.T) {
				get := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "http://attacker.invalid"+path, nil)
				req.Header.Set("Forwarded", "host=attacker.invalid;proto=http")
				req.Header.Set("X-Forwarded-Host", "attacker.invalid")
				handler.ServeHTTP(get, req)
				require.Equal(t, http.StatusOK, get.Code)
				switch path {
				case "/robots.txt":
					assert.Equal(t, "text/plain; charset=utf-8", get.Header().Get("Content-Type"))
					assert.Equal(t, "User-agent: *\nAllow: /\n\nSitemap: "+base+"sitemap.xml\n", get.Body.String())
				case "/sitemap.xml":
					assert.Equal(t, "application/xml; charset=utf-8", get.Header().Get("Content-Type"))
					var document struct {
						XMLName xml.Name
						URLs    []string `xml:"url>loc"`
					}
					require.NoError(t, xml.Unmarshal(get.Body.Bytes(), &document))
					assert.Equal(t, "http://www.sitemaps.org/schemas/sitemap/0.9", document.XMLName.Space)
					assert.Equal(t, []string{base + "ui/", base + "ui/workbench/", base + "ui/examples/", base + "ui/connect/", base + "ui/privacy/"}, document.URLs)
				default:
					assert.Empty(t, get.Header().Get("X-Robots-Tag"), "public HTML may be indexed")
					page := get.Body.String()
					canonical := base + strings.TrimPrefix(path, "/")
					assert.Contains(t, page, `<link rel="canonical" href="`+canonical+`">`)
					assert.Contains(t, page, `<meta property="og:url" content="`+canonical+`">`)
					assert.Contains(t, page, `<meta name="twitter:card" content="summary">`)
					assert.NotContains(t, page, "{{")
					assert.NotContains(t, strings.Split(page, "</head>")[0], "attacker.invalid")
				}
				head := httptest.NewRecorder()
				headRequest := req.Clone(req.Context())
				headRequest.Method = http.MethodHead
				handler.ServeHTTP(head, headRequest)
				assert.Equal(t, http.StatusOK, head.Code)
				assert.Empty(t, head.Body.Bytes())
				assert.Equal(t, get.Header(), head.Header())
				post := httptest.NewRecorder()
				handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, path, nil))
				assert.Equal(t, http.StatusMethodNotAllowed, post.Code)
				assert.Equal(t, "GET, HEAD", post.Header().Get("Allow"))
			})
		}
	}
}

func TestDiscoveryDefaultsAndDuplicateURLs(t *testing.T) {
	for _, base := range []string{"", "https://echo.container.education/"} {
		handler := NewHandler(&config.Config{PublicURL: base}, "HTTP")
		for _, path := range []string{"/ui/?sensitive=sample", "/ui/workbench/?example=websocket", "/ui/examples/?anything=1", "/ui/connect/?anything=1", "/ui/privacy/?anything=1", "/ui/config.json", "/ui/certificate.json", "/ui/app.js", "/ui/missing", "/ui/metadata.html"} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			assert.Equal(t, "noindex", response.Header().Get("X-Robots-Tag"), path)
			if strings.HasSuffix(path, "missing") || strings.HasSuffix(path, "metadata.html") {
				assert.Equal(t, http.StatusNotFound, response.Code)
			} else {
				assert.Equal(t, http.StatusOK, response.Code)
			}
		}
	}
	private := NewHandler(&config.Config{}, "HTTP")
	for _, path := range []string{"/robots.txt", "/ui/", "/ui/workbench/", "/ui/examples/", "/ui/connect/", "/ui/privacy/"} {
		response := httptest.NewRecorder()
		private.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, "noindex", response.Header().Get("X-Robots-Tag"))
		assert.NotContains(t, response.Body.String(), `rel="canonical"`)
		if path == "/robots.txt" {
			assert.Equal(t, "User-agent: *\nDisallow: /\n", response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	private.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
	assert.Equal(t, http.StatusNotFound, response.Code)
}

func TestReflectedRequestDetailsAreExcludedFromSearchSnippets(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://private-search-marker.invalid/ui/?sample=private-search-marker", nil)
	request.Header.Set("X-Sample", "private-search-marker")
	NewHandler(&config.Config{PublicURL: "https://echo.container.education/", PrintHeaders: true}, "HTTP").ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	document, err := html.Parse(response.Body)
	require.NoError(t, err)
	var inspect func(*html.Node, bool)
	markers := 0
	inspect = func(node *html.Node, excluded bool) {
		for _, attr := range node.Attr {
			if attr.Key == "data-nosnippet" {
				excluded = true
			}
		}
		if node.Type == html.TextNode && strings.Contains(node.Data, "private-search-marker") {
			markers++
			assert.True(t, excluded, "reflected details must stay inside data-nosnippet: %s", node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			inspect(child, excluded)
		}
	}
	inspect(document, false)
	assert.Greater(t, markers, 4, "test must cover both request JSON and layer details")
}
