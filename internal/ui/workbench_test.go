package ui

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkbenchEmptyStatesUseCurrentLogoAndExplainDestination(t *testing.T) {
	response := httptest.NewRecorder()
	NewHandler(&config.Config{}, "HTTP").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/workbench/", nil))
	require.Equal(t, http.StatusOK, response.Code)
	page := response.Body.String()
	for _, id := range []string{"response-empty", "websocket-empty"} {
		placeholder := regexp.MustCompile(`(?s)<div id="` + id + `".*?</div>`).FindString(page)
		require.NotEmpty(t, placeholder)
		assert.Contains(t, placeholder, `<img class="empty-logo" src="../logo.svg" alt="" width="160" height="146">`)
	}
	assert.NotContains(t, page, "echo-aperture")
	assert.NotContains(t, page, "Same service")
	assert.Contains(t, page, `<details id="request-destination-help"`)
	assert.Contains(t, page, `aria-describedby="request-destination-description"`)
	assert.Contains(t, page, `id="request-destination-address"`)
	assert.Contains(t, page, "Paths stay within this address.")
}

func TestWorkbenchHasHeaderRowsEditor(t *testing.T) {
	response := httptest.NewRecorder()
	NewHandler(&config.Config{}, "HTTP").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ui/workbench/", nil))
	require.Equal(t, http.StatusOK, response.Code)
	page := response.Body.String()
	assert.Contains(t, page, `<fieldset class="header-editor" aria-describedby="headers-hint">`)
	assert.Contains(t, page, `<legend>Headers</legend>`)
	assert.Contains(t, page, `<div id="request-headers"></div>`)
	assert.Contains(t, page, `<button id="add-header" type="button"`)
	assert.NotContains(t, page, `Headers <span class="label-note">JSON object</span>`)
}
