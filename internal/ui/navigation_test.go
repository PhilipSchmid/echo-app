package ui

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNavigationPreservesLabelsAndCurrentPage(t *testing.T) {
	handler := NewHandler(&config.Config{}, "HTTP")
	for _, page := range []struct{ path, current, href string }{
		{"/ui/", "request", "./"},
		{"/ui/workbench/", "workbench", "../workbench/"},
		{"/ui/examples/", "examples", "../examples/"},
		{"/ui/connect/", "connect", "../connect/"},
	} {
		t.Run(page.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, page.path, nil))
			require.Equal(t, http.StatusOK, response.Code)
			nav := regexp.MustCompile(`(?s)<nav class="primary-nav".*?</nav>`).FindString(response.Body.String())
			require.NotEmpty(t, nav)
			assert.Contains(t, nav, `aria-label="Main navigation"`)
			assert.Equal(t, 4, strings.Count(nav, "<a "))
			assert.Equal(t, 4, strings.Count(nav, `aria-hidden="true"`))
			assert.Equal(t, 1, strings.Count(nav, `aria-current="page"`))
			assert.Contains(t, nav, `href="`+page.href+`" aria-current="page" class="nav-`+page.current+`"`)
			nav = strings.ReplaceAll(nav, "<wbr>", "")
			previous := -1
			for _, label := range []string{"Your request", "Workbench", "Examples", "Connect"} {
				position := strings.Index(nav, "<span>"+label+"</span>")
				require.Greater(t, position, previous, "navigation order: %s", label)
				previous = position
			}
		})
	}
}

func TestRequestAndWorkbenchPagesStaySeparate(t *testing.T) {
	handler := NewHandler(&config.Config{}, "HTTP")
	for _, page := range []struct {
		path    string
		present []string
		absent  []string
	}{
		{
			path:    "/ui/",
			present: []string{`id="visit-body"`, `id="connection-stack"`, `src="./visit.js"`},
			absent:  []string{`id="request-form"`, `id="websocket-form"`, `id="workbench"`, `src="./app.js"`},
		},
		{
			path:    "/ui/workbench/",
			present: []string{`id="request-form"`, `id="websocket-form"`, `src="../app.js"`, `href="../examples/"`, "Browse examples"},
			absent:  []string{`id="visit-body"`, `id="connection-stack"`, `src="../visit.js"`, "{{.Request"},
		},
	} {
		t.Run(page.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, page.path, nil))
			require.Equal(t, http.StatusOK, response.Code)
			for _, value := range page.present {
				assert.Contains(t, response.Body.String(), value)
			}
			for _, value := range page.absent {
				assert.NotContains(t, response.Body.String(), value)
			}
			if page.path == "/ui/workbench/" {
				assert.Regexp(t, `<h1\b[^>]*>Request workbench</h1>`, response.Body.String())
				assert.Less(t, strings.Index(response.Body.String(), "Browse examples"), strings.Index(response.Body.String(), `id="request-form"`), "examples shortcut belongs above the request form")
			}
		})
	}
}
