//go:build !windows && !linux

package core

import (
	"context"
	"fmt"
	"runtime"
)

// runNeighborCommand reports the unsupported platform instead of failing the
// whole scan: IPv4 discovery still runs.
func runNeighborCommand(_ context.Context) (string, error) {
	return "", fmt.Errorf("ipv6 neighbor discovery is not supported on %s", runtime.GOOS)
}
