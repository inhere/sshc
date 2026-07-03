package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultSSHPort = 22
	DefaultGroup   = "default"
	ConfigEnvKey   = "SSHC_CONFIG"
)

var userHomeDir = os.UserHomeDir

type Host struct {
	Name     string `json:"name"`
	IP       string `json:"ip"`
	User     string `json:"user"`
	Password string `json:"password"`
	KeyPath  string `json:"key_path,omitempty"`
	Remark   string `json:"remark,omitempty"`
	Group    string `json:"group,omitempty"`
	Port     int    `json:"port"`
}

type Store struct {
	Hosts []Host `json:"hosts"`
}

func (s *Store) Upsert(host Host) error {
	if err := validateHost(host); err != nil {
		return err
	}
	for i, item := range s.Hosts {
		if item.Name == host.Name || item.IP == host.IP {
			s.Hosts[i] = host
			return nil
		}
	}
	s.Hosts = append(s.Hosts, host)
	return nil
}

func (s Store) Find(target string) (Host, bool) {
	target = strings.TrimSpace(target)
	for _, host := range s.Hosts {
		if host.Name == target || host.IP == target {
			return host, true
		}
	}
	return Host{}, false
}

func (s Store) ResolveHost(target string) (Host, bool, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return Host{}, false, nil
	}
	if host, ok := s.Find(target); ok {
		return host, true, nil
	}

	matches := s.MatchHosts(target)
	switch len(matches) {
	case 0:
		return Host{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return Host{}, false, fmt.Errorf("host %q matches multiple hosts: %s", target, formatHostCandidates(matches))
	}
}

func (s Store) MatchHosts(target string) []Host {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(target)))
	if len(parts) == 0 {
		return nil
	}

	var matches []Host
	for _, host := range s.Hosts {
		text := strings.ToLower(strings.Join([]string{
			strings.TrimSpace(host.Name),
			strings.TrimSpace(host.IP),
			strings.TrimSpace(host.Remark),
			strings.TrimSpace(HostGroupName(host)),
		}, " "))
		if matchAllParts(text, parts) {
			matches = append(matches, host)
		}
	}
	return matches
}

func matchAllParts(text string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

func formatHostCandidates(hosts []Host) string {
	items := make([]string, 0, len(hosts))
	for _, host := range hosts {
		name := HostLogName(host)
		items = append(items, fmt.Sprintf("%s (%s:%d)", name, host.IP, host.Port))
	}
	return strings.Join(items, ", ")
}

func validateHost(host Host) error {
	if strings.TrimSpace(host.IP) == "" {
		return errors.New("ip is required")
	}
	if strings.TrimSpace(host.User) == "" {
		return errors.New("user is required")
	}
	if host.Password == "" && strings.TrimSpace(host.KeyPath) == "" {
		return errors.New("password or key_path is required")
	}
	if host.Port < 1 || host.Port > 65535 {
		return fmt.Errorf("invalid ssh port %d", host.Port)
	}
	if strings.Contains(host.IP, ":") {
		if _, _, err := net.SplitHostPort(host.IP); err == nil {
			return errors.New("ip should not include port, use --port")
		}
	}
	return nil
}

func HostGroupName(host Host) string {
	if strings.TrimSpace(host.Group) != "" {
		return strings.TrimSpace(host.Group)
	}
	return DefaultGroup
}

func LoadStore() (*Store, error) {
	path, err := StorePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Store{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return &Store{}, nil
	}

	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	return &store, nil
}

func LoadStoreWithSSHConfig() (*Store, error) {
	store, err := LoadStore()
	if err != nil {
		return nil, err
	}
	hosts, err := LoadSSHConfigHosts()
	if err != nil {
		return nil, err
	}
	for _, host := range hosts {
		if _, ok := store.Find(host.Name); ok {
			continue
		}
		if _, ok := store.Find(host.IP); ok {
			continue
		}
		store.Hosts = append(store.Hosts, host)
	}
	return store, nil
}

func LoadSSHConfigHosts() ([]Host, error) {
	configPath, err := sshConfigPath()
	if err != nil {
		return nil, err
	}
	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	return ParseSSHConfig(file)
}

func ParseSSHConfig(reader io.Reader) ([]Host, error) {
	return parseSSHConfig(reader)
}

func parseSSHConfig(reader io.Reader) ([]Host, error) {
	scanner := bufio.NewScanner(reader)
	var hosts []Host
	var current *Host
	flush := func() {
		if current == nil {
			return
		}
		normalizeSSHConfigHost(current)
		if current.Name != "" && current.IP != "" && current.User != "" && current.KeyPath != "" {
			hosts = append(hosts, *current)
		}
		current = nil
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.ToLower(fields[0])
		values := fields[1:]
		if key == "host" {
			flush()
			if len(values) != 1 || hasSSHHostPattern(values[0]) {
				current = nil
				continue
			}
			current = &Host{Name: values[0], Port: DefaultSSHPort, Group: "ssh-config"}
			continue
		}
		if current == nil {
			continue
		}
		value := strings.Join(values, " ")
		switch key {
		case "hostname":
			current.IP = value
		case "user":
			current.User = value
		case "port":
			if port, err := strconv.Atoi(value); err == nil {
				current.Port = port
			}
		case "identityfile":
			if current.KeyPath == "" {
				current.KeyPath = value
			}
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return hosts, nil
}

func normalizeSSHConfigHost(host *Host) {
	host.Name = strings.TrimSpace(host.Name)
	host.IP = strings.TrimSpace(host.IP)
	host.User = strings.TrimSpace(host.User)
	host.KeyPath = strings.TrimSpace(host.KeyPath)
	if host.IP == "" {
		host.IP = host.Name
	}
	if host.User == "" {
		host.User = currentUserName()
	}
	if host.Group == "" {
		host.Group = "ssh-config"
	}
	if host.Port == 0 {
		host.Port = DefaultSSHPort
	}
}

func hasSSHHostPattern(value string) bool {
	return strings.ContainsAny(value, "*?!")
}

func currentUserName() string {
	if user := strings.TrimSpace(os.Getenv("USER")); user != "" {
		return user
	}
	return strings.TrimSpace(os.Getenv("USERNAME"))
}

func sshConfigPath() (string, error) {
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

func SaveStore(store *Store) error {
	path, err := StorePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func StorePath() (string, error) {
	if path := strings.TrimSpace(os.Getenv(ConfigEnvKey)); path != "" {
		return path, nil
	}
	dir, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hosts.json"), nil
}

func SetUserHomeDirForTest(fn func() (string, error)) func() {
	old := userHomeDir
	userHomeDir = fn
	return func() { userHomeDir = old }
}
