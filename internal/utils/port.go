package utils

import "strconv"

// IsValidPort checks if a port is valid
func IsValidPort(port string) bool {
	p, err := strconv.Atoi(port)
	return err == nil && p > 0 && p <= 65535
}
