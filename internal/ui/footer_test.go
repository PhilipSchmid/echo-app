package ui

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedFooterIsDecorativeAndPreservesProjectLinks(t *testing.T) {
	handler := NewHandler(&config.Config{}, "HTTP")
	for _, path := range []string{"/ui/", "/ui/workbench/", "/ui/examples/", "/ui/connect/", "/ui/privacy/"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusOK, response.Code)
			page := response.Body.String()
			footers := regexp.MustCompile(`(?s)<footer\b[^>]*>.*?</footer>`).FindAllString(page, -1)
			require.Len(t, footers, 1)
			footer := footers[0]
			assert.Contains(t, footer, `class="site-footer"`)
			assert.Regexp(t, `<span class="footer-version"><span class="sr-only">Version </span>[^<]+</span>`, footer)
			assert.Contains(t, footer, template.HTMLEscapeString(applicationVersion()))
			assert.Regexp(t, `<span class="brand-echo">Echo</span>\s+<span class="brand-observatory">Observatory</span>`, footer)
			assert.NotContains(t, footer, "Small requests.")
			assert.NotContains(t, footer, "Clear answers.")
			assert.NotContains(t, footer, "footer-caption")
			assert.Contains(t, footer, `aria-label="Project links"`)
			for href, label := range map[string]string{
				"https://github.com/philipschmid/echo-observatory":                   "Source code",
				"https://github.com/philipschmid/echo-observatory/tree/main/docs":    "Docs",
				"https://github.com/philipschmid/echo-observatory/blob/main/LICENSE": "Apache 2.0",
			} {
				assert.Regexp(t, `(?s)<a\b[^>]*href="`+regexp.QuoteMeta(href)+`"[^>]*>.*?`+regexp.QuoteMeta(label)+`.*?</a>`, footer)
			}
			assert.Regexp(t, `<[^>]*class="footer-scene"[^>]*aria-hidden="true"`, footer)
			image := regexp.MustCompile(`<img\b[^>]*>`).FindString(footer)
			require.NotEmpty(t, image)
			logo := "../logo.svg"
			if path == "/ui/" {
				logo = "./logo.svg"
			}
			for _, attr := range []string{`src="` + logo + `"`, `alt=""`, `width="128"`, `height="117"`} {
				assert.Contains(t, image, attr)
			}
			privacy := "../privacy/"
			if path == "/ui/" {
				privacy = "./privacy/"
			}
			assert.Contains(t, footer, `href="`+privacy+`"`)
			assert.Contains(t, footer, `class="footer-privacy"`)
			assert.NotContains(t, page, "Requests reach this server and may appear in server logs.")
			assert.NotContains(t, page, "{{")
		})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/footer.html", nil))
	assert.Equal(t, http.StatusNotFound, response.Code)
}

func TestFooterVersionIsEscapedText(t *testing.T) {
	const label = `<script>alert("version")</script>`
	page := renderPage("footer", pageView{Version: label})
	require.NoError(t, page.err)
	assert.Contains(t, string(page.data), template.HTMLEscapeString(label))
	assert.NotContains(t, string(page.data), "<script>")
}

func TestFooterUsesStampedBuildVersion(t *testing.T) {
	previous := version
	version = "v2.1.1"
	t.Cleanup(func() { version = previous })
	handler := NewHandler(&config.Config{}, "HTTP")
	for _, path := range []string{"/ui/", "/ui/workbench/", "/ui/examples/", "/ui/connect/", "/ui/privacy/"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, response.Code)
		assert.Contains(t, response.Body.String(), `<span class="footer-version"><span class="sr-only">Version </span>v2.1.1</span>`)
	}
}
