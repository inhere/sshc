package core

import "strings"

const MaskedSecret = "***"

func MaskConfig(config Config) Config {
	if len(config.AuthProfiles) > 0 {
		profiles := make([]AuthProfile, len(config.AuthProfiles))
		for i, profile := range config.AuthProfiles {
			profiles[i] = MaskAuthProfile(profile)
		}
		config.AuthProfiles = profiles
	}
	if len(config.Hosts) > 0 {
		hosts := make([]Host, len(config.Hosts))
		for i, host := range config.Hosts {
			hosts[i] = MaskHost(host)
		}
		config.Hosts = hosts
	}
	return config
}

func MaskHost(host Host) Host {
	if host.Password != "" || host.PasswordEnc != "" {
		host.Password = ""
		host.PasswordEnc = MaskedSecret
	}
	return host
}

func MaskAuthProfile(profile AuthProfile) AuthProfile {
	if profile.Password != "" || profile.PasswordEnc != "" {
		profile.Password = ""
		profile.PasswordEnc = MaskedSecret
	}
	return profile
}

func AuthLabel(host Host) string {
	if ref := strings.TrimSpace(host.AuthRef); ref != "" {
		return "auth:" + ref
	}
	hasKey := strings.TrimSpace(host.KeyPath) != ""
	hasPassword := host.Password != "" || host.PasswordEnc != ""
	switch {
	case hasKey && hasPassword:
		return "key+password"
	case hasKey:
		return "key"
	case hasPassword:
		return "password"
	default:
		return "-"
	}
}
