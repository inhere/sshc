package core

import (
	"fmt"
	"strings"
)

const (
	DoctorOK    = "ok"
	DoctorWarn  = "warn"
	DoctorError = "error"
)

type DoctorIssue struct {
	Level   string `json:"level"`
	Item    string `json:"item"`
	Message string `json:"message"`
}

func CheckConfig(config Config) []DoctorIssue {
	var issues []DoctorIssue
	issues = append(issues, checkHostDuplicates(config.Hosts)...)
	issues = append(issues, checkAuthDuplicates(config.AuthProfiles)...)
	issues = append(issues, checkAuthRefs(config)...)
	issues = append(issues, checkHostPorts(config.Hosts)...)
	issues = append(issues, checkGroupDefaults(config)...)
	issues = append(issues, checkHostKeyPolicy(config)...)
	issues = append(issues, checkCommandProxyHosts(config)...)
	if len(issues) == 0 {
		return []DoctorIssue{{Level: DoctorOK, Item: "config", Message: "config looks valid"}}
	}
	return issues
}

func HasDoctorErrors(issues []DoctorIssue) bool {
	for _, issue := range issues {
		if issue.Level == DoctorError {
			return true
		}
	}
	return false
}

func checkHostDuplicates(hosts []Host) []DoctorIssue {
	var issues []DoctorIssue
	names := map[string]bool{}
	ips := map[string]bool{}
	for _, host := range hosts {
		name := strings.TrimSpace(host.Name)
		if name != "" {
			if names[name] {
				issues = append(issues, DoctorIssue{Level: DoctorError, Item: "hosts", Message: fmt.Sprintf("duplicate host name %q", name)})
			}
			names[name] = true
		}
		ip := strings.TrimSpace(host.IP)
		if ip != "" {
			if ips[ip] {
				issues = append(issues, DoctorIssue{Level: DoctorError, Item: "hosts", Message: fmt.Sprintf("duplicate host ip %q", ip)})
			}
			ips[ip] = true
		}
	}
	return issues
}

func checkAuthDuplicates(profiles []AuthProfile) []DoctorIssue {
	var issues []DoctorIssue
	names := map[string]bool{}
	for _, profile := range profiles {
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "auth_profiles", Message: "auth profile name is required"})
			continue
		}
		if names[name] {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "auth_profiles", Message: fmt.Sprintf("duplicate auth profile name %q", name)})
		}
		names[name] = true
	}
	return issues
}

func checkAuthRefs(config Config) []DoctorIssue {
	var issues []DoctorIssue
	profiles := map[string]bool{}
	for _, profile := range config.AuthProfiles {
		if name := strings.TrimSpace(profile.Name); name != "" {
			profiles[name] = true
		}
	}
	for _, host := range config.Hosts {
		if IsCommandProxyHost(host) {
			continue
		}
		ref := strings.TrimSpace(host.AuthRef)
		if ref == "" {
			continue
		}
		if !profiles[ref] {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "hosts", Message: fmt.Sprintf("host %q references missing auth profile %q", HostLogName(host), ref)})
		}
	}
	for name, group := range config.Groups {
		ref := strings.TrimSpace(group.AuthRef)
		if ref == "" {
			continue
		}
		if !profiles[ref] {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "groups", Message: fmt.Sprintf("group %q references missing auth profile %q", strings.TrimSpace(name), ref)})
		}
	}
	return issues
}

func checkHostPorts(hosts []Host) []DoctorIssue {
	var issues []DoctorIssue
	for _, host := range hosts {
		if host.Port == 0 {
			continue
		}
		if host.Port < 1 || host.Port > 65535 {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "hosts", Message: fmt.Sprintf("host %q has invalid port %d", HostLogName(host), host.Port)})
		}
	}
	return issues
}

func checkGroupDefaults(config Config) []DoctorIssue {
	var issues []DoctorIssue
	store := storeFromConfig(config)
	for name, group := range config.Groups {
		name = strings.TrimSpace(name)
		if name == "" {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "groups", Message: "group name is required"})
			continue
		}
		if group.Port < 0 || group.Port > 65535 {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "groups", Message: fmt.Sprintf("group %q has invalid port %d", name, group.Port)})
		}
		jump := strings.TrimSpace(group.Jump)
		if jump != "" {
			if _, ok, err := store.ResolveHost(jump); err != nil {
				issues = append(issues, DoctorIssue{Level: DoctorError, Item: "groups", Message: fmt.Sprintf("group %q jump %q is ambiguous: %v", name, jump, err)})
			} else if !ok {
				issues = append(issues, DoctorIssue{Level: DoctorError, Item: "groups", Message: fmt.Sprintf("group %q references missing jump host %q", name, jump)})
			}
		}
	}
	return issues
}

func checkCommandProxyHosts(config Config) []DoctorIssue {
	var issues []DoctorIssue
	store := storeFromConfig(config)
	for _, host := range config.Hosts {
		if err := validateHostBackend(host); err != nil {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "hosts", Message: fmt.Sprintf("host %q has %s", HostLogName(host), err.Error())})
			continue
		}
		if !IsCommandProxyHost(host) {
			if strings.TrimSpace(host.Via) != "" || strings.TrimSpace(host.RunTemplate) != "" || strings.TrimSpace(host.LoginCommand) != "" {
				issues = append(issues, DoctorIssue{Level: DoctorWarn, Item: "hosts", Message: fmt.Sprintf("host %q has command_proxy fields but backend is ssh", HostLogName(host))})
			}
			continue
		}
		name := HostLogName(host)
		viaName := strings.TrimSpace(host.Via)
		if viaName == "" {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "hosts", Message: fmt.Sprintf("command_proxy host %q requires via", name)})
		}
		if strings.TrimSpace(host.RunTemplate) == "" && strings.TrimSpace(host.LoginCommand) == "" {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "hosts", Message: fmt.Sprintf("command_proxy host %q requires run_template or login_command", name)})
		}
		if err := ValidateCommandProxyTemplate(host); err != nil {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "hosts", Message: err.Error()})
		}
		if viaName == "" {
			continue
		}
		via, ok, err := store.ResolveHost(viaName)
		if err != nil {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "hosts", Message: fmt.Sprintf("command_proxy host %q via %q is ambiguous: %v", name, viaName, err)})
			continue
		}
		if !ok {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "hosts", Message: fmt.Sprintf("command_proxy host %q references missing via host %q", name, viaName)})
			continue
		}
		if sameHostIdentity(host, via) || strings.TrimSpace(host.Name) == strings.TrimSpace(via.Name) {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "hosts", Message: fmt.Sprintf("command_proxy host %q cannot use itself as via", name)})
		}
		if IsCommandProxyHost(via) {
			issues = append(issues, DoctorIssue{Level: DoctorError, Item: "hosts", Message: fmt.Sprintf("command_proxy host %q via host %q is also command_proxy", name, HostLogName(via))})
		}
	}
	return issues
}

func checkHostKeyPolicy(config Config) []DoctorIssue {
	var issues []DoctorIssue
	check := func(item, owner, value string) {
		value = strings.TrimSpace(value)
		if value == "" || value == HostKeyCheckKnownHosts || value == HostKeyCheckInsecure {
			return
		}
		issues = append(issues, DoctorIssue{Level: DoctorError, Item: item, Message: fmt.Sprintf("%s has invalid host_key_check %q", owner, value)})
	}
	check("defaults", "defaults", config.Defaults.HostKeyCheck)
	for name, group := range config.Groups {
		check("groups", fmt.Sprintf("group %q", strings.TrimSpace(name)), group.HostKeyCheck)
	}
	for _, host := range config.Hosts {
		check("hosts", fmt.Sprintf("host %q", HostLogName(host)), host.HostKeyCheck)
	}
	return issues
}
