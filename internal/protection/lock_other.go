//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package protection

import (
	"fmt"
	"os"
)

func lockState(_ string) (*os.File, error) {
	return nil, fmt.Errorf("persistent transfer budgets are unsupported on this platform; omit transfer-state-file to use a per-process budget")
}
