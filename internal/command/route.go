package command

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/inhere/sshc/internal/core"
)

type commandRoute struct {
	session core.ForwardSession
}

func (r *commandRoute) Close() error {
	if r == nil || r.session == nil {
		return nil
	}
	return r.session.Close()
}

func resolveCommandRoute(host core.Host, viaLocalPort int, viaJump, explicitJump string, noHostKeyPrompt, trustNewHost bool) (core.Host, *commandRoute, error) {
	if viaLocalPort < 0 || viaLocalPort > 65535 {
		return core.Host{}, nil, fmt.Errorf("--via-local-port must be between 1 and 65535")
	}
	if viaLocalPort > 0 && strings.TrimSpace(viaJump) != "" {
		return core.Host{}, nil, errorsRoute("--via-local-port and --via-jump cannot be used together")
	}
	if (viaLocalPort > 0 || strings.TrimSpace(viaJump) != "") && strings.TrimSpace(explicitJump) != "" {
		return core.Host{}, nil, errorsRoute("--via-local-port/--via-jump cannot be used with --jump")
	}
	if (viaLocalPort > 0 || strings.TrimSpace(viaJump) != "") && strings.TrimSpace(host.Jump) != "" {
		return core.Host{}, nil, errorsRoute("explicit local routing cannot be used with a saved jump; remove the saved jump or omit via routing")
	}

	host.NoHostKeyPrompt = noHostKeyPrompt
	host.TrustNewHost = trustNewHost
	if viaLocalPort == 0 && strings.TrimSpace(viaJump) == "" {
		return host, nil, nil
	}
	if viaLocalPort > 0 {
		host.DialAddress = net.JoinHostPort("127.0.0.1", strconv.Itoa(viaLocalPort))
		return host, nil, nil
	}

	config, err := core.LoadConfigWithSSHConfig()
	if err != nil {
		return core.Host{}, nil, err
	}
	jumpEffective, ok, err := config.ResolveEffectiveHost(strings.TrimSpace(viaJump), core.HostOverrides{})
	if err != nil {
		return core.Host{}, nil, err
	}
	if !ok {
		return core.Host{}, nil, fmt.Errorf("via jump host %q not found", viaJump)
	}
	jump := jumpEffective.ToHost()
	if strings.TrimSpace(jump.Jump) != "" {
		return core.Host{}, nil, fmt.Errorf("via jump host %q has its own jump; nested jumps are not supported", viaJump)
	}
	remoteAddress := net.JoinHostPort(strings.TrimSpace(host.IP), strconv.Itoa(effectivePort(host)))
	session, err := core.StartLocalForward(jump, []core.ForwardRule{{
		LocalAddr:  "127.0.0.1:0",
		RemoteAddr: remoteAddress,
	}}, core.ForwardOptions{
		NoHostKeyPrompt: noHostKeyPrompt,
		TrustNewHost:    trustNewHost,
	})
	if err != nil {
		return core.Host{}, nil, err
	}
	endpoints := session.Endpoints()
	if len(endpoints) != 1 || strings.TrimSpace(endpoints[0]) == "" {
		if closeErr := session.Close(); closeErr != nil {
			return core.Host{}, nil, fmt.Errorf("via jump did not create a local endpoint; close: %w", closeErr)
		}
		return core.Host{}, nil, fmt.Errorf("via jump did not create a local endpoint")
	}
	host.DialAddress = endpoints[0]
	return host, &commandRoute{session: session}, nil
}

func effectivePort(host core.Host) int {
	if host.Port > 0 {
		return host.Port
	}
	return core.DefaultSSHPort
}

func errorsRoute(message string) error {
	return fmt.Errorf("invalid connection route: %s", message)
}
