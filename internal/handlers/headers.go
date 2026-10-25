package handlers

import "strings"

// diagnosticHeaders copies values so redaction never changes the incoming request.
func diagnosticHeaders(headers map[string][]string, unsafe bool) map[string][]string {
	result := make(map[string][]string, len(headers))
	for name, values := range headers {
		lower := strings.ToLower(name)
		if !unsafe && (lower == "authorization" || lower == "proxy-authorization" ||
			lower == "cookie" || lower == "set-cookie" || strings.Contains(lower, "token") ||
			strings.Contains(lower, "secret") || strings.Contains(lower, "key") ||
			strings.Contains(lower, "password") || strings.Contains(lower, "passphrase") ||
			strings.Contains(lower, "credential")) {
			result[name] = []string{"[REDACTED]"}
		} else {
			result[name] = append([]string(nil), values...)
		}
	}
	return result
}
