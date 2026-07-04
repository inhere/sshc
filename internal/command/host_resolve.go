package command

import (
	"fmt"

	"github.com/inhere/sshc/internal/core"
)

func resolveCommandHost(target string) (core.Host, error) {
	host, ok, err := core.ResolveHostWithSSHConfig(target, core.HostOverrides{})
	if err != nil {
		return core.Host{}, err
	}
	if !ok {
		return core.Host{}, fmt.Errorf("host %q not found", target)
	}
	return host, nil
}
