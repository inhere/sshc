//go:build windows

package core

import (
	"context"
	"fmt"
	"os/exec"
)

// runNeighborCommand reads the IPv6 neighbor cache through netsh.
func runNeighborCommand(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "netsh", "interface", "ipv6", "show", "neighbors").Output()
	if err != nil {
		return "", fmt.Errorf("read ipv6 neighbors via netsh: %w", err)
	}
	return string(out), nil
}
