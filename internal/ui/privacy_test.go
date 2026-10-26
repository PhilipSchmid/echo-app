package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PhilipSchmid/echo-observatory/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivacyNoticeDescribesInstanceWithoutReflectingRequestData(t *testing.T) {
	for _, tc := range []struct {
		name            string
		headers, unsafe bool
		policy          string
	}{
		{"default redaction", true, false, "Common credential headers are also redacted in responses."},
		{"unsafe headers", true, true, "Response credential redaction is disabled on this instance."},
		{"unsafe without header output", false, true, "Request headers are not included in responses on this instance."},
		{"no header output", false, false, "Request headers are not included in responses on this instance."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/ui/privacy/?sample=private-marker", nil)
			request.Header.Set("Authorization", "Bearer private-marker")
			response := httptest.NewRecorder()
			NewHandler(&config.Config{PrintHeaders: tc.headers, UnsafeHeaders: tc.unsafe}, "HTTP").ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
			page := response.Body.String()
			assert.Contains(t, page, "<h1>Privacy &amp; data</h1>")
			assert.Contains(t, page, tc.policy)
			assert.Contains(t, page, "session storage")
			assert.Contains(t, page, "Philip Schmid")
			assert.Contains(t, page, "hosted by Fly.io, Inc., with the application running in Germany")
			for _, policyURL := range []string{
				"https://fly.io/legal/privacy-policy/",
				"https://fly.io/legal/sub-processors/",
				"https://fly.io/legal/data-privacy-framework/",
			} {
				assert.Contains(t, page, `<a href="`+policyURL+`">`)
			}
			assert.Contains(t, page, "seven days")
			assert.Contains(t, page, "Common credential headers are always redacted in diagnostic logs.")
			assert.NotContains(t, page, "Obtain those details from its operator")
			assert.Contains(t, page, `<a href="mailto:privacy@container.education">privacy@container.education</a>`)
			for _, detail := range []string{"Article 6(1)(f)", "portability", "object to processing", "ten minutes", "transfer safeguards", "Privacy correspondence", "Swiss FDPIC"} {
				assert.Contains(t, page, detail)
			}
			assert.NotContains(t, page, "docs/privacy.md", "the website must contain the complete notice")
			assert.NotContains(t, page, "private-marker")
			assert.Empty(t, response.Header().Values("Set-Cookie"))
		})
	}
}
