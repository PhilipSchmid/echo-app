package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidPort(t *testing.T) {
	tests := []struct {
		name     string
		port     string
		expected bool
	}{
		{
			name:     "valid port 80",
			port:     "80",
			expected: true,
		},
		{
			name:     "valid port 8080",
			port:     "8080",
			expected: true,
		},
		{
			name:     "valid port 65535",
			port:     "65535",
			expected: true,
		},
		{
			name:     "invalid port 0",
			port:     "0",
			expected: false,
		},
		{
			name:     "invalid port negative",
			port:     "-1",
			expected: false,
		},
		{
			name:     "invalid port too high",
			port:     "65536",
			expected: false,
		},
		{
			name:     "invalid port not a number",
			port:     "abc",
			expected: false,
		},
		{
			name:     "invalid empty port",
			port:     "",
			expected: false,
		},
		{
			name:     "invalid port with spaces",
			port:     " 80 ",
			expected: false,
		},
		{
			name:     "invalid port decimal",
			port:     "80.5",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidPort(tt.port)
			assert.Equal(t, tt.expected, result)
		})
	}
}
