package core

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type BatchHostSource struct {
	Hosts     []string
	HostsFile string
	Group     string
	Overrides HostOverrides
	AuthRef   string
	AllowRaw  bool
}

func ReadHostsFile(path string) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("hosts file is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var hosts []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		hosts = append(hosts, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return hosts, nil
}

func ResolveBatchHosts(source BatchHostSource) ([]Host, error) {
	targets, group, err := batchTargets(source)
	if err != nil {
		return nil, err
	}
	config, err := LoadConfigWithSSHConfig()
	if err != nil {
		return nil, err
	}
	if group != "" {
		return resolveBatchGroup(*config, source, group)
	}
	return resolveBatchTargets(*config, source, targets)
}

func batchTargets(source BatchHostSource) ([]string, string, error) {
	sourceCount := 0
	if len(source.Hosts) > 0 {
		sourceCount++
	}
	if strings.TrimSpace(source.HostsFile) != "" {
		sourceCount++
	}
	if strings.TrimSpace(source.Group) != "" {
		sourceCount++
	}
	if sourceCount != 1 {
		return nil, "", errors.New("exactly one of --hosts, --hosts-file, or --group is required")
	}
	if strings.TrimSpace(source.Group) != "" {
		return nil, strings.TrimSpace(source.Group), nil
	}
	targets := source.Hosts
	if strings.TrimSpace(source.HostsFile) != "" {
		var err error
		targets, err = ReadHostsFile(source.HostsFile)
		if err != nil {
			return nil, "", err
		}
	}
	targets, err := normalizeBatchTargets(targets)
	if err != nil {
		return nil, "", err
	}
	if len(targets) == 0 {
		return nil, "", errors.New("no hosts found")
	}
	return targets, "", nil
}

func normalizeBatchTargets(values []string) ([]string, error) {
	var targets []string
	for _, value := range values {
		parts := strings.Split(value, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				return nil, errors.New("empty host target is not allowed")
			}
			targets = append(targets, part)
		}
	}
	return targets, nil
}

func resolveBatchTargets(config Config, source BatchHostSource, targets []string) ([]Host, error) {
	store := storeFromConfig(config)
	seen := map[string]struct{}{}
	hosts := make([]Host, 0, len(targets))
	for _, target := range targets {
		host, ok, err := store.ResolveHost(target)
		if err != nil {
			return nil, err
		}
		if !ok {
			if !source.AllowRaw {
				return nil, fmt.Errorf("host %q not found", target)
			}
			host, err = resolveRawBatchHost(config, source, target)
			if err != nil {
				return nil, err
			}
		} else {
			effective, _, err := config.EffectiveHost(host, source.Overrides)
			if err != nil {
				return nil, err
			}
			host = effective.ToHost()
		}
		if addUniqueBatchHost(&hosts, seen, host) {
			continue
		}
	}
	return hosts, nil
}

func resolveBatchGroup(config Config, source BatchHostSource, group string) ([]Host, error) {
	group = strings.TrimSpace(group)
	seen := map[string]struct{}{}
	var hosts []Host
	for _, host := range config.Hosts {
		if HostGroupName(host) != group {
			continue
		}
		effective, _, err := config.EffectiveHost(host, source.Overrides)
		if err != nil {
			return nil, err
		}
		addUniqueBatchHost(&hosts, seen, effective.ToHost())
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no hosts found in group %q", group)
	}
	return hosts, nil
}

func resolveRawBatchHost(config Config, source BatchHostSource, target string) (Host, error) {
	target = strings.TrimSpace(target)
	if err := validateRawBatchTarget(target); err != nil {
		return Host{}, err
	}
	raw := Host{
		Name:     target,
		IP:       target,
		AuthRef:  strings.TrimSpace(source.AuthRef),
		User:     strings.TrimSpace(source.Overrides.User),
		Password: source.Overrides.Password,
		KeyPath:  strings.TrimSpace(source.Overrides.KeyPath),
		Port:     source.Overrides.Port,
	}
	effective, _, err := config.EffectiveHost(raw, source.Overrides)
	if err != nil {
		return Host{}, err
	}
	return effective.ToHost(), nil
}

func validateRawBatchTarget(target string) error {
	if target == "" {
		return errors.New("raw host target is required")
	}
	if strings.ContainsAny(target, " \t\r\n") {
		return fmt.Errorf("invalid raw host target %q", target)
	}
	if strings.Contains(target, ":") {
		return fmt.Errorf("raw host target %q should not include port, use --port", target)
	}
	if ip := net.ParseIP(target); ip != nil {
		return nil
	}
	if !isValidRawHostname(target) {
		return fmt.Errorf("invalid raw host target %q", target)
	}
	return nil
}

func isValidRawHostname(value string) bool {
	if len(value) > 253 {
		return false
	}
	labels := strings.Split(value, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return false
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func addUniqueBatchHost(hosts *[]Host, seen map[string]struct{}, host Host) bool {
	key := batchHostKey(host)
	if _, ok := seen[key]; ok {
		return false
	}
	seen[key] = struct{}{}
	*hosts = append(*hosts, host)
	return true
}

func batchHostKey(host Host) string {
	ip := strings.ToLower(strings.TrimSpace(host.IP))
	if ip != "" {
		return ip + ":" + strconv.Itoa(host.Port)
	}
	return strings.ToLower(HostLogName(host))
}
