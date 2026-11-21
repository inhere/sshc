//go:build linux

package core

import (
	"context"
	"fmt"
	"os/exec"
)

// runNeighborCommand reads the IPv6 neighbor cache through iproute2.
func runNeighborCommand(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "ip", "-6", "neigh", "show").Output()
	if err != nil {
		return "", fmt.Errorf("read ipv6 neighbors via ip: %w", err)
	}
	return string(out), nil
}
