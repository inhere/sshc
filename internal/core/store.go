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
	DefaultSSHPort       = 22
	DefaultGroup         = "default"
	ConfigVersion        = 1
	ConfigEnvKey         = "SSHC_CONFIG"
	ConfigFileName       = "sshc.config.json"
	LegacyConfigFileName = "hosts.json"
)

var userHomeDir = os.UserHomeDir

type Host struct {
	Name        string `json:"name"`
	IP          string `json:"ip"`
	AuthRef     string `json:"auth_ref,omitempty"`
	User        string `json:"user"`
	Password    string `json:"password,omitempty"`
	PasswordEnc string `json:"password_enc,omitempty"`
	KeyPath     string `json:"key_path,omitempty"`
	Remark      string `json:"remark,omitempty"`
	Group       string `json:"group,omitempty"`
	Port        int    `json:"port,omitempty"`
	Jump        string `json:"jump,omitempty"`

	ConnectTimeout  string `json:"connect_timeout,omitempty"`
	RunTimeout      string `json:"run_timeout,omitempty"`
	RemoteScriptDir string `json:"remote_script_dir,omitempty"`
	HostKeyCheck    string `json:"host_key_check,omitempty"`
	KnownHostsPath  string `json:"known_hosts_path,omitempty"`
}

type Store struct {
	LogsPath string `json:"logs_path,omitempty"`
	Hosts    []Host `json:"hosts"`
}

type Defaults struct {
	User            string `json:"user,omitempty"`
	Port            int    `json:"port,omitempty"`
	ConnectTimeout  string `json:"connect_timeout,omitempty"`
	RunTimeout      string `json:"run_timeout,omitempty"`
	RemoteScriptDir string `json:"remote_script_dir,omitempty"`
	HostKeyCheck    string `json:"host_key_check,omitempty"`
	KnownHostsPath  string `json:"known_hosts_path,omitempty"`
}

type AuthProfile struct {
	Name        string `json:"name"`
	User        string `json:"user,omitempty"`
	Password    string `json:"password,omitempty"`
	PasswordEnc string `json:"password_enc,omitempty"`
	KeyPath     string `json:"key_path,omitempty"`
	Remark      string `json:"remark,omitempty"`
}

type Config struct {
	Version      int           `json:"version"`
	LogsPath     string        `json:"logs_path,omitempty"`
	Defaults     Defaults      `json:"defaults,omitempty"`
	AuthProfiles []AuthProfile `json:"auth_profiles"`
	Hosts        []Host        `json:"hosts"`
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
	if strings.TrimSpace(host.User) == "" && strings.TrimSpace(host.AuthRef) == "" {
		return errors.New("user is required")
	}
	if strings.TrimSpace(host.AuthRef) == "" && host.Password == "" && host.PasswordEnc == "" && strings.TrimSpace(host.KeyPath) == "" {
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
	config, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	store := storeFromConfig(*config)
	return &store, nil
}

func LoadConfig() (*Config, error) {
	path, data, err := readConfigFile()
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return newEmptyConfig(), nil
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	normalizeConfig(&config)
	if err := decryptConfigPasswords(&config); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	return &config, nil
}

func LoadStoreWithSSHConfig() (*Store, error) {
	config, err := LoadConfigWithSSHConfig()
	if err != nil {
		return nil, err
	}
	store := storeFromConfig(*config)
	return &store, nil
}

func LoadConfigWithSSHConfig() (*Config, error) {
	config, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	hosts, err := LoadSSHConfigHosts()
	if err != nil {
		return nil, err
	}
	for _, host := range hosts {
		store := storeFromConfig(*config)
		if _, ok := store.Find(host.Name); ok {
			continue
		}
		if _, ok := store.Find(host.IP); ok {
			continue
		}
		config.Hosts = append(config.Hosts, host)
	}
	return config, nil
}

func LoadConfigSettings() (Store, error) {
	config, err := LoadConfig()
	if err != nil {
		return Store{}, err
	}
	return storeFromConfig(*config), nil
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
	config := configFromStore(*store)
	return SaveConfig(&config)
}

func SaveConfig(config *Config) error {
	if config == nil {
		return errors.New("config is nil")
	}
	path, err := StorePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	saveConfig, err := encryptConfigPasswords(*config)
	if err != nil {
		return err
	}
	normalizeConfigForSave(&saveConfig)
	data, err := json.MarshalIndent(saveConfig, "", "  ")
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

func newEmptyConfig() *Config {
	return &Config{
		Version:      ConfigVersion,
		AuthProfiles: []AuthProfile{},
		Hosts:        []Host{},
	}
}

func normalizeConfig(config *Config) {
	if config.Version == 0 {
		config.Version = ConfigVersion
	}
	if config.AuthProfiles == nil {
		config.AuthProfiles = []AuthProfile{}
	}
	if config.Hosts == nil {
		config.Hosts = []Host{}
	}
}

func normalizeConfigForSave(config *Config) {
	config.Version = ConfigVersion
	normalizeConfig(config)
}

func storeFromConfig(config Config) Store {
	return Store{
		LogsPath: strings.TrimSpace(config.LogsPath),
		Hosts:    config.Hosts,
	}
}

func configFromStore(store Store) Config {
	return Config{
		Version:      ConfigVersion,
		LogsPath:     strings.TrimSpace(store.LogsPath),
		AuthProfiles: []AuthProfile{},
		Hosts:        store.Hosts,
	}
}

func StorePath() (string, error) {
	if path := strings.TrimSpace(os.Getenv(ConfigEnvKey)); path != "" {
		return path, nil
	}
	dir, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFileName), nil
}

func LegacyStorePath() (string, error) {
	dir, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, LegacyConfigFileName), nil
}

func readConfigFile() (string, []byte, error) {
	path, err := StorePath()
	if err != nil {
		return "", nil, err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		return path, data, nil
	}
	if !os.IsNotExist(err) {
		return path, nil, err
	}
	if strings.TrimSpace(os.Getenv(ConfigEnvKey)) != "" {
		return path, nil, nil
	}

	legacyPath, legacyErr := LegacyStorePath()
	if legacyErr != nil {
		return "", nil, legacyErr
	}
	data, legacyErr = os.ReadFile(legacyPath)
	if legacyErr == nil {
		return legacyPath, data, nil
	}
	if os.IsNotExist(legacyErr) {
		return path, nil, nil
	}
	return legacyPath, nil, legacyErr
}

func ReadConfigFile() (string, []byte, error) {
	return readConfigFile()
}

func SetUserHomeDirForTest(fn func() (string, error)) func() {
	old := userHomeDir
	userHomeDir = fn
	return func() { userHomeDir = old }
}
