package core

import (
	"fmt"
	"strings"
)

const (
	HostKeyCheckKnownHosts = "known_hosts"
	HostKeyCheckInsecure   = "insecure"

	DefaultConnectTimeout  = "20s"
	DefaultRemoteScriptDir = "/tmp"
	DefaultKnownHostsPath  = "~/.ssh/known_hosts"
)

type HostOverrides struct {
	User            string
	Password        string
	KeyPath         string
	Port            int
	ConnectTimeout  string
	RunTimeout      string
	RemoteScriptDir string
	HostKeyCheck    string
	KnownHostsPath  string
}

type ResolveConnectionOptions struct {
	Jump string
}

type EffectiveHost struct {
	Host
	ConnectTimeout  string
	RunTimeout      string
	RemoteScriptDir string
	HostKeyCheck    string
	KnownHostsPath  string
}

type ResolvedConnection struct {
	Target Host
	Jump   *Host
}

func (h EffectiveHost) ToHost() Host {
	host := h.Host
	host.User = strings.TrimSpace(h.User)
	host.Password = h.Password
	host.PasswordEnc = h.PasswordEnc
	host.KeyPath = strings.TrimSpace(h.KeyPath)
	host.Port = h.Port
	host.ConnectTimeout = strings.TrimSpace(h.ConnectTimeout)
	host.RunTimeout = strings.TrimSpace(h.RunTimeout)
	host.RemoteScriptDir = strings.TrimSpace(h.RemoteScriptDir)
	host.HostKeyCheck = strings.TrimSpace(h.HostKeyCheck)
	host.KnownHostsPath = strings.TrimSpace(h.KnownHostsPath)
	return host
}

func (c Config) ResolveEffectiveHost(target string, overrides HostOverrides) (EffectiveHost, bool, error) {
	store := storeFromConfig(c)
	host, ok, err := store.ResolveHost(target)
	if err != nil || !ok {
		return EffectiveHost{}, ok, err
	}
	return c.EffectiveHost(host, overrides)
}

func ResolveHostWithSSHConfig(target string, overrides HostOverrides) (Host, bool, error) {
	config, err := LoadConfigWithSSHConfig()
	if err != nil {
		return Host{}, false, err
	}
	host, ok, err := config.ResolveEffectiveHost(target, overrides)
	if err != nil || !ok {
		return Host{}, ok, err
	}
	return host.ToHost(), true, nil
}

func ResolveConnectionWithSSHConfig(target string, opts ResolveConnectionOptions) (ResolvedConnection, bool, error) {
	config, err := LoadConfigWithSSHConfig()
	if err != nil {
		return ResolvedConnection{}, false, err
	}
	effective, ok, err := config.ResolveEffectiveHost(target, HostOverrides{})
	if err != nil || !ok {
		return ResolvedConnection{}, ok, err
	}
	host := effective.ToHost()
	if jump := strings.TrimSpace(opts.Jump); jump != "" {
		host.Jump = jump
	}
	conn, err := config.ResolveConnection(host)
	if err != nil {
		return ResolvedConnection{}, false, err
	}
	return conn, true, nil
}

func ResolveConnectionForHost(host Host) (ResolvedConnection, error) {
	config, err := LoadConfigWithSSHConfig()
	if err != nil {
		return ResolvedConnection{}, err
	}
	return config.ResolveConnection(host)
}

func (c Config) EffectiveHost(host Host, overrides HostOverrides) (EffectiveHost, bool, error) {
	effective := EffectiveHost{
		Host:            host,
		ConnectTimeout:  DefaultConnectTimeout,
		RemoteScriptDir: DefaultRemoteScriptDir,
		HostKeyCheck:    HostKeyCheckKnownHosts,
		KnownHostsPath:  DefaultKnownHostsPath,
	}
	applyDefaults(&effective, c.Defaults)

	if !IsCommandProxyHost(host) {
		if ref := strings.TrimSpace(host.AuthRef); ref != "" {
			profile, ok := c.FindAuthProfile(ref)
			if !ok {
				return EffectiveHost{}, false, fmt.Errorf("auth profile %q not found for host %q", ref, HostLogName(host))
			}
			applyAuthProfile(&effective, profile)
		}
	}

	applyHostInline(&effective, host)
	applyOverrides(&effective, overrides)
	if effective.Port == 0 {
		effective.Port = DefaultSSHPort
	}

	if err := validateEffectiveHost(effective); err != nil {
		return EffectiveHost{}, false, err
	}
	return effective, true, nil
}

func (c Config) ResolveConnection(host Host) (ResolvedConnection, error) {
	if IsCommandProxyHost(host) {
		return ResolvedConnection{}, fmt.Errorf("host %q uses command_proxy backend; ssh connection is not supported", HostLogName(host))
	}
	conn := ResolvedConnection{Target: host}
	jumpName := strings.TrimSpace(host.Jump)
	if jumpName == "" {
		return conn, nil
	}

	jumpEffective, ok, err := c.ResolveEffectiveHost(jumpName, HostOverrides{})
	if err != nil {
		return ResolvedConnection{}, err
	}
	if !ok {
		return ResolvedConnection{}, fmt.Errorf("jump host %q not found for host %q", jumpName, HostLogName(host))
	}
	jump := jumpEffective.ToHost()
	if sameHostIdentity(host, jump) {
		return ResolvedConnection{}, fmt.Errorf("host %q cannot jump through itself", HostLogName(host))
	}
	if nested := strings.TrimSpace(jump.Jump); nested != "" {
		return ResolvedConnection{}, fmt.Errorf("jump host %q also has jump %q; multi-level jump is not supported", HostLogName(jump), nested)
	}
	conn.Jump = &jump
	return conn, nil
}

func (c Config) FindAuthProfile(name string) (AuthProfile, bool) {
	name = strings.TrimSpace(name)
	for _, profile := range c.AuthProfiles {
		if strings.TrimSpace(profile.Name) == name {
			return profile, true
		}
	}
	return AuthProfile{}, false
}

func applyDefaults(host *EffectiveHost, defaults Defaults) {
	if value := strings.TrimSpace(defaults.User); value != "" {
		host.User = value
	}
	if defaults.Port > 0 {
		host.Port = defaults.Port
	}
	if value := strings.TrimSpace(defaults.ConnectTimeout); value != "" {
		host.ConnectTimeout = value
	}
	if value := strings.TrimSpace(defaults.RunTimeout); value != "" {
		host.RunTimeout = value
	}
	if value := strings.TrimSpace(defaults.RemoteScriptDir); value != "" {
		host.RemoteScriptDir = value
	}
	if value := strings.TrimSpace(defaults.HostKeyCheck); value != "" {
		host.HostKeyCheck = value
	}
	if value := strings.TrimSpace(defaults.KnownHostsPath); value != "" {
		host.KnownHostsPath = value
	}
}

func applyAuthProfile(host *EffectiveHost, profile AuthProfile) {
	if value := strings.TrimSpace(profile.User); value != "" {
		host.User = value
	}
	if profile.Password != "" {
		host.Password = profile.Password
	}
	if profile.PasswordEnc != "" {
		host.PasswordEnc = profile.PasswordEnc
	}
	if value := strings.TrimSpace(profile.KeyPath); value != "" {
		host.KeyPath = value
	}
}

func applyHostInline(effective *EffectiveHost, host Host) {
	if value := strings.TrimSpace(host.User); value != "" {
		effective.User = value
	}
	if host.Password != "" {
		effective.Password = host.Password
	}
	if host.PasswordEnc != "" {
		effective.PasswordEnc = host.PasswordEnc
	}
	if value := strings.TrimSpace(host.KeyPath); value != "" {
		effective.KeyPath = value
	}
	if host.Port > 0 {
		effective.Port = host.Port
	}
	if value := strings.TrimSpace(host.ConnectTimeout); value != "" {
		effective.ConnectTimeout = value
	}
	if value := strings.TrimSpace(host.RunTimeout); value != "" {
		effective.RunTimeout = value
	}
	if value := strings.TrimSpace(host.RemoteScriptDir); value != "" {
		effective.RemoteScriptDir = value
	}
	if value := strings.TrimSpace(host.HostKeyCheck); value != "" {
		effective.HostKeyCheck = value
	}
	if value := strings.TrimSpace(host.KnownHostsPath); value != "" {
		effective.KnownHostsPath = value
	}
}

func applyOverrides(host *EffectiveHost, overrides HostOverrides) {
	if value := strings.TrimSpace(overrides.User); value != "" {
		host.User = value
	}
	if overrides.Password != "" {
		host.Password = overrides.Password
	}
	if value := strings.TrimSpace(overrides.KeyPath); value != "" {
		host.KeyPath = value
	}
	if overrides.Port > 0 {
		host.Port = overrides.Port
	}
	if value := strings.TrimSpace(overrides.ConnectTimeout); value != "" {
		host.ConnectTimeout = value
	}
	if value := strings.TrimSpace(overrides.RunTimeout); value != "" {
		host.RunTimeout = value
	}
	if value := strings.TrimSpace(overrides.RemoteScriptDir); value != "" {
		host.RemoteScriptDir = value
	}
	if value := strings.TrimSpace(overrides.HostKeyCheck); value != "" {
		host.HostKeyCheck = value
	}
	if value := strings.TrimSpace(overrides.KnownHostsPath); value != "" {
		host.KnownHostsPath = value
	}
}

func sameHostIdentity(a, b Host) bool {
	aName := strings.TrimSpace(a.Name)
	bName := strings.TrimSpace(b.Name)
	if aName != "" && bName != "" && aName == bName {
		return true
	}
	aIP := strings.TrimSpace(a.IP)
	bIP := strings.TrimSpace(b.IP)
	if aIP != "" && bIP != "" && aIP == bIP && effectivePort(a) == effectivePort(b) {
		return true
	}
	return false
}

func effectivePort(host Host) int {
	if host.Port > 0 {
		return host.Port
	}
	return DefaultSSHPort
}

func validateEffectiveHost(host EffectiveHost) error {
	if IsCommandProxyHost(host.Host) {
		return validateHost(host.Host)
	}
	if strings.TrimSpace(host.IP) == "" {
		return fmt.Errorf("ip is required")
	}
	if strings.TrimSpace(host.User) == "" {
		return fmt.Errorf("user is required for host %q", HostLogName(host.Host))
	}
	if host.Password == "" && host.PasswordEnc == "" && strings.TrimSpace(host.KeyPath) == "" {
		return fmt.Errorf("password or key_path is required for host %q", HostLogName(host.Host))
	}
	if host.Port < 1 || host.Port > 65535 {
		return fmt.Errorf("invalid ssh port %d", host.Port)
	}
	switch strings.TrimSpace(host.HostKeyCheck) {
	case HostKeyCheckKnownHosts, HostKeyCheckInsecure:
		return nil
	default:
		return fmt.Errorf("invalid host_key_check %q, want known_hosts or insecure", host.HostKeyCheck)
	}
}
