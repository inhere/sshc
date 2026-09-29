package core

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// TunnelForward is the persisted form of one local port forwarding rule.
// It is converted to the runtime ForwardRule through TunnelProfile.ForwardRules.
type TunnelForward struct {
	Local  string `json:"local"`
	Remote string `json:"remote"`
}

// TunnelProfile is a named, persisted port forwarding configuration. It targets
// either a saved host (Target) or an unregistered address (Address, requires AuthRef).
type TunnelProfile struct {
	Name     string          `json:"name"`
	Target   string          `json:"target,omitempty"`
	Address  string          `json:"address,omitempty"`
	Port     int             `json:"port,omitempty"`
	Jump     string          `json:"jump,omitempty"`
	AuthRef  string          `json:"auth_ref,omitempty"`
	Forwards []TunnelForward `json:"forwards"`
	Remark   string          `json:"remark,omitempty"`
}

// ForwardRules converts the persisted rules into runtime forward rules.
func (p TunnelProfile) ForwardRules() ([]ForwardRule, error) {
	rules := make([]ForwardRule, 0, len(p.Forwards))
	for _, item := range p.Forwards {
		local, err := normalizeForwardEndpoint(item.Local, true)
		if err != nil {
			return nil, fmt.Errorf("invalid forward rule %q: %w", forwardLabel(item), err)
		}
		remote, err := normalizeForwardEndpoint(item.Remote, false)
		if err != nil {
			return nil, fmt.Errorf("invalid forward rule %q: %w", forwardLabel(item), err)
		}
		rules = append(rules, ForwardRule{LocalAddr: local, RemoteAddr: remote})
	}
	return rules, nil
}

// ParseForwardRule parses "local[host:]port=remote[host:]port" into its canonical form.
func ParseForwardRule(value string) (TunnelForward, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return TunnelForward{}, errors.New("forward rule is required")
	}
	if strings.Count(value, "=") != 1 {
		return TunnelForward{}, fmt.Errorf("invalid forward rule %q, want local=remote (eg: 15432=127.0.0.1:5432)", value)
	}
	localText, remoteText, _ := strings.Cut(value, "=")
	local, err := normalizeForwardEndpoint(localText, true)
	if err != nil {
		return TunnelForward{}, fmt.Errorf("invalid local endpoint %q: %w", strings.TrimSpace(localText), err)
	}
	remote, err := normalizeForwardEndpoint(remoteText, false)
	if err != nil {
		return TunnelForward{}, fmt.Errorf("invalid remote endpoint %q: %w", strings.TrimSpace(remoteText), err)
	}
	return TunnelForward{Local: local, Remote: remote}, nil
}

// NormalizeTunnelProfile trims and canonicalizes a profile for storage. It never
// fails: malformed rules are kept (trimmed) so that config doctor can report them.
func NormalizeTunnelProfile(profile *TunnelProfile) {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Target = strings.TrimSpace(profile.Target)
	profile.Address = strings.TrimSpace(profile.Address)
	profile.Jump = strings.TrimSpace(profile.Jump)
	profile.AuthRef = strings.TrimSpace(profile.AuthRef)
	profile.Remark = strings.TrimSpace(profile.Remark)
	if profile.Forwards == nil {
		profile.Forwards = []TunnelForward{}
	}
	for i := range profile.Forwards {
		local := strings.TrimSpace(profile.Forwards[i].Local)
		remote := strings.TrimSpace(profile.Forwards[i].Remote)
		if normalized, err := normalizeForwardEndpoint(local, true); err == nil {
			local = normalized
		}
		if normalized, err := normalizeForwardEndpoint(remote, false); err == nil {
			remote = normalized
		}
		profile.Forwards[i] = TunnelForward{Local: local, Remote: remote}
	}
}

// ValidateTunnelProfile applies the tunnel add hard checks (no global write gate).
func ValidateTunnelProfile(cfg Config, profile TunnelProfile) error {
	if err := validateTunnelName(profile.Name); err != nil {
		return err
	}
	target := strings.TrimSpace(profile.Target)
	address := strings.TrimSpace(profile.Address)
	switch {
	case target == "" && address == "":
		return errors.New("tunnel target or address is required")
	case target != "" && address != "":
		return errors.New("tunnel target and address are mutually exclusive")
	}
	if address != "" && strings.TrimSpace(profile.AuthRef) == "" {
		return fmt.Errorf("tunnel %q uses an address target, --auth is required", profile.Name)
	}
	if ref := strings.TrimSpace(profile.AuthRef); ref != "" {
		if _, ok := cfg.FindAuthProfile(ref); !ok {
			return fmt.Errorf("auth profile %q not found", ref)
		}
	}
	if profile.Port != 0 && (profile.Port < 1 || profile.Port > 65535) {
		return fmt.Errorf("invalid tunnel port %d, want 1-65535", profile.Port)
	}
	if jump := strings.TrimSpace(profile.Jump); jump != "" {
		store := Store{Hosts: cfg.Hosts}
		if _, ok := store.Find(jump); !ok {
			return fmt.Errorf("jump host %q not found", jump)
		}
	}
	if len(profile.Forwards) == 0 {
		return fmt.Errorf("tunnel %q requires at least one --forward rule", profile.Name)
	}
	seen := map[string]string{}
	for _, item := range profile.Forwards {
		local, err := normalizeForwardEndpoint(item.Local, true)
		if err != nil {
			return fmt.Errorf("invalid forward rule %q: %w", forwardLabel(item), err)
		}
		if _, err := normalizeForwardEndpoint(item.Remote, false); err != nil {
			return fmt.Errorf("invalid forward rule %q: %w", forwardLabel(item), err)
		}
		host, portText, err := net.SplitHostPort(local)
		if err != nil {
			return fmt.Errorf("invalid local endpoint %q: %w", local, err)
		}
		if !isLoopbackEndpointHost(host) {
			return fmt.Errorf("local endpoint %q must bind a loopback address (127.0.0.1 or [::1])", local)
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			return fmt.Errorf("invalid local port %q", portText)
		}
		if port == 0 {
			continue
		}
		if first, ok := seen[local]; ok {
			return fmt.Errorf("duplicate local endpoint %q (already used by %s)", local, first)
		}
		seen[local] = local
	}
	return nil
}

// CanonicalTunnelTarget resolves a save-time host target to its canonical host name.
// Fuzzy matching is allowed here (same UX as sshc run); the stored form is exact.
func CanonicalTunnelTarget(cfg Config, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("host target is required")
	}
	store := Store{Hosts: cfg.Hosts}
	host, ok, err := store.ResolveHost(target)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("host %q not found; use --address with --auth to save an unregistered address", target)
	}
	return host.Name, nil
}

// ResolveTunnelHost builds the effective SSH host for a tunnel profile.
//
// Host mode requires an exact match (Store.Find semantics): a renamed or deleted
// host must fail loudly instead of silently degrading into an unregistered address.
// Only address mode may resolve an unregistered target.
func ResolveTunnelHost(cfg Config, profile TunnelProfile) (Host, error) {
	label := tunnelLabel(profile)
	target := strings.TrimSpace(profile.Target)
	address := strings.TrimSpace(profile.Address)
	authRef := strings.TrimSpace(profile.AuthRef)
	switch {
	case target == "" && address == "":
		return Host{}, fmt.Errorf("tunnel %q target or address is required", label)
	case target != "" && address != "":
		return Host{}, fmt.Errorf("tunnel %q target and address are mutually exclusive", label)
	}

	resolved := address
	if target != "" {
		store := Store{Hosts: cfg.Hosts}
		host, ok := store.Find(target)
		if !ok {
			return Host{}, fmt.Errorf("tunnel %q target host %q not found; fix the host or save an --address target", label, target)
		}
		resolved = host.Name
	} else if authRef == "" {
		return Host{}, fmt.Errorf("tunnel %q uses an address target, --auth is required", label)
	}

	effective, _, err := cfg.ResolveEffectiveHostWithAuth(resolved, authRef)
	if err != nil {
		return Host{}, fmt.Errorf("tunnel %q: %w", label, err)
	}
	host := effective.ToHost()
	if IsCommandProxyHost(host) {
		return Host{}, fmt.Errorf("tunnel %q target %q uses command_proxy backend; local port forwarding is not supported", label, HostLogName(host))
	}
	if profile.Port > 0 {
		host.Port = profile.Port
	}
	if jump := strings.TrimSpace(profile.Jump); jump != "" {
		host.Jump = jump
	}
	return host, nil
}

// FindTunnel returns the tunnel profile with the given name.
func FindTunnel(profiles []TunnelProfile, name string) (TunnelProfile, bool) {
	name = strings.TrimSpace(name)
	for _, profile := range profiles {
		if strings.TrimSpace(profile.Name) == name {
			return profile, true
		}
	}
	return TunnelProfile{}, false
}

// UpsertTunnel adds or replaces a tunnel profile. Replacing requires force.
func UpsertTunnel(profiles []TunnelProfile, profile TunnelProfile, force bool) ([]TunnelProfile, error) {
	name := strings.TrimSpace(profile.Name)
	for i, item := range profiles {
		if strings.TrimSpace(item.Name) != name {
			continue
		}
		if !force {
			return profiles, fmt.Errorf("tunnel %q already exists, use --force to update it", name)
		}
		updated := make([]TunnelProfile, len(profiles))
		copy(updated, profiles)
		updated[i] = profile
		return updated, nil
	}
	updated := make([]TunnelProfile, 0, len(profiles)+1)
	updated = append(updated, profiles...)
	return append(updated, profile), nil
}

// RemoveTunnel removes a tunnel profile by name.
func RemoveTunnel(profiles []TunnelProfile, name string) []TunnelProfile {
	name = strings.TrimSpace(name)
	kept := make([]TunnelProfile, 0, len(profiles))
	for _, profile := range profiles {
		if strings.TrimSpace(profile.Name) == name {
			continue
		}
		kept = append(kept, profile)
	}
	return kept
}

// TunnelsUsingAuth lists tunnels that reference the given auth profile.
func TunnelsUsingAuth(profiles []TunnelProfile, authName string) []string {
	authName = strings.TrimSpace(authName)
	if authName == "" {
		return nil
	}
	var names []string
	for _, profile := range profiles {
		if strings.TrimSpace(profile.AuthRef) == authName {
			names = append(names, tunnelLabel(profile))
		}
	}
	return names
}

// TunnelsUsingHost lists tunnels that reference the given host as target or jump.
func TunnelsUsingHost(profiles []TunnelProfile, hostName string) []string {
	hostName = strings.TrimSpace(hostName)
	if hostName == "" {
		return nil
	}
	var names []string
	for _, profile := range profiles {
		if strings.TrimSpace(profile.Target) == hostName || strings.TrimSpace(profile.Jump) == hostName {
			names = append(names, tunnelLabel(profile))
		}
	}
	return names
}

func validateTunnelName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("tunnel name is required")
	}
	if strings.ContainsAny(name, " \t\r\n/\\") {
		return fmt.Errorf("invalid tunnel name %q", name)
	}
	return nil
}

func tunnelLabel(profile TunnelProfile) string {
	if name := strings.TrimSpace(profile.Name); name != "" {
		return name
	}
	return "unnamed"
}

func forwardLabel(item TunnelForward) string {
	local := strings.TrimSpace(item.Local)
	remote := strings.TrimSpace(item.Remote)
	if local == "" && remote == "" {
		return "empty"
	}
	return local + "=" + remote
}

// normalizeForwardEndpoint, parseForwardPort, validateForwardPortNumber,
// validateForwardEndpointHost and isLoopbackEndpointHost live in forward.go so that
// the forward core compiles independently of the tunnel profile model.
