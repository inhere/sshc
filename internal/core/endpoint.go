package core

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// parsedHostTarget is the normalized form of a raw command target. A port is
// present only when the input used an explicit host:port grammar.
type parsedHostTarget struct {
	Host    string
	Zone    string
	Port    int
	HasPort bool
}

func parseHostTarget(value string) (parsedHostTarget, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return parsedHostTarget{}, fmt.Errorf("target is empty")
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return parsedHostTarget{}, fmt.Errorf("target %q contains whitespace", value)
	}

	if strings.HasPrefix(value, "[") {
		close := strings.IndexByte(value, ']')
		if close < 0 || close == 1 || close+1 >= len(value) || value[close+1] != ':' {
			return parsedHostTarget{}, fmt.Errorf("target %q, want [ipv6%%zone]:port", value)
		}
		if strings.Contains(value[close+1:], "[") || strings.Contains(value[close+1:], "]") {
			return parsedHostTarget{}, fmt.Errorf("target %q has invalid brackets", value)
		}
		host := value[1:close]
		addr, err := netip.ParseAddr(host)
		if err != nil || !addr.Is6() {
			return parsedHostTarget{}, fmt.Errorf("target %q has invalid IPv6 address", value)
		}
		if err := validateTargetZone(addr.Zone()); err != nil {
			return parsedHostTarget{}, fmt.Errorf("target %q: %w", value, err)
		}
		port, err := parseSSHPort(value[close+2:])
		if err != nil {
			return parsedHostTarget{}, fmt.Errorf("target %q: %w", value, err)
		}
		return parsedHostTarget{Host: host, Zone: addr.Zone(), Port: port, HasPort: true}, nil
	}
	if strings.ContainsAny(value, "[]") {
		return parsedHostTarget{}, fmt.Errorf("target %q has invalid brackets", value)
	}

	if addr, err := netip.ParseAddr(value); err == nil {
		if err := validateTargetZone(addr.Zone()); err != nil {
			return parsedHostTarget{}, fmt.Errorf("target %q: %w", value, err)
		}
		return parsedHostTarget{Host: value, Zone: addr.Zone()}, nil
	}

	if strings.Count(value, ":") == 1 {
		host, portText, err := net.SplitHostPort(value)
		if err != nil {
			return parsedHostTarget{}, fmt.Errorf("target %q, want host:port", value)
		}
		if err := validateTargetHost(host); err != nil {
			return parsedHostTarget{}, fmt.Errorf("target %q: %w", value, err)
		}
		port, err := parseSSHPort(portText)
		if err != nil {
			return parsedHostTarget{}, fmt.Errorf("target %q: %w", value, err)
		}
		return parsedHostTarget{Host: host, Port: port, HasPort: true}, nil
	}
	if strings.Contains(value, ":") {
		return parsedHostTarget{}, fmt.Errorf("target %q, IPv6 port must use [host]:port", value)
	}
	if err := validateTargetHost(value); err != nil {
		return parsedHostTarget{}, err
	}
	return parsedHostTarget{Host: value}, nil
}

func validateTargetZone(zone string) error {
	if zone == "" {
		return nil
	}
	if strings.ContainsAny(zone, ":[]% \t\r\n") {
		return fmt.Errorf("invalid zone %q", zone)
	}
	return nil
}

func validateTargetHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("host is empty")
	}
	if addr, err := netip.ParseAddr(host); err == nil && addr.IsValid() {
		return nil
	}
	if !isValidRawHostname(host) {
		return fmt.Errorf("invalid host %q", host)
	}
	return nil
}

func parseSSHPort(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("port is required")
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q, want 1-65535", value)
	}
	return port, nil
}

func validateRawTarget(value string) error {
	parsed, err := parseHostTarget(value)
	if err != nil {
		return err
	}
	if parsed.HasPort {
		return fmt.Errorf("target %q should not include port, use --port", strings.TrimSpace(value))
	}
	return nil
}
