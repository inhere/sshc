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
	"sort"
	"strconv"
	"strings"
)

const (
	DefaultSSHPort  = 22
	DefaultGroup    = "default"
	ConfigVersion   = 1
	ConfigEnvKey    = "SSHC_CONFIG"
	ConfigDirEnvKey = "SSHC_CONFIG_DIR"
	ConfigFileName  = "sshc.config.json"

	HostBackendSSH          = "ssh"
	HostBackendCommandProxy = "command_proxy"
	CommandProxyCmdToken    = "{{cmd}}"
)

var userHomeDir = os.UserHomeDir

type Host struct {
	Name             string   `json:"name"`
	IP               string   `json:"ip"`
	AuthRef          string   `json:"auth_ref,omitempty"`
	User             string   `json:"user"`
	Password         string   `json:"password,omitempty"`
	PasswordEnc      string   `json:"password_enc,omitempty"`
	KeyPath          string   `json:"key_path,omitempty"`
	KeyData          string   `json:"key_data,omitempty"`
	KeyDataEnc       string   `json:"key_data_enc,omitempty"`
	KeyPassphrase    string   `json:"key_passphrase,omitempty"`
	KeyPassphraseEnc string   `json:"key_passphrase_enc,omitempty"`
	Remark           string   `json:"remark,omitempty"`
	Group            string   `json:"group,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	Port             int      `json:"port,omitempty"`
	Jump             string   `json:"jump,omitempty"`
	Backend          string   `json:"backend,omitempty"`
	Via              string   `json:"via,omitempty"`
	RunTemplate      string   `json:"run_template,omitempty"`
	LoginCommand     string   `json:"login_command,omitempty"`

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

type GroupDefaults struct {
	AuthRef         string `json:"auth_ref,omitempty"`
	User            string `json:"user,omitempty"`
	KeyPath         string `json:"key_path,omitempty"`
	Port            int    `json:"port,omitempty"`
	Jump            string `json:"jump,omitempty"`
	ConnectTimeout  string `json:"connect_timeout,omitempty"`
	RunTimeout      string `json:"run_timeout,omitempty"`
	RemoteScriptDir string `json:"remote_script_dir,omitempty"`
	HostKeyCheck    string `json:"host_key_check,omitempty"`
	KnownHostsPath  string `json:"known_hosts_path,omitempty"`
}

type AuthProfile struct {
	Name             string `json:"name"`
	User             string `json:"user,omitempty"`
	Password         string `json:"password,omitempty"`
	PasswordEnc      string `json:"password_enc,omitempty"`
	KeyPath          string `json:"key_path,omitempty"`
	KeyData          string `json:"key_data,omitempty"`
	KeyDataEnc       string `json:"key_data_enc,omitempty"`
	KeyPassphrase    string `json:"key_passphrase,omitempty"`
	KeyPassphraseEnc string `json:"key_passphrase_enc,omitempty"`
	Remark           string `json:"remark,omitempty"`
}

type Config struct {
	Version      int                      `json:"version"`
	LogsPath     string                   `json:"logs_path,omitempty"`
	Defaults     Defaults                 `json:"defaults,omitempty"`
	Groups       map[string]GroupDefaults `json:"groups,omitempty"`
	AuthProfiles []AuthProfile            `json:"auth_profiles"`
	Hosts        []Host                   `json:"hosts"`
}

func (s *Store) Upsert(host Host) error {
	if err := validateHost(host); err != nil {
		return err
	}
	for i, item := range s.Hosts {
		if item.Name == host.Name || (strings.TrimSpace(item.IP) != "" && strings.TrimSpace(item.IP) == strings.TrimSpace(host.IP)) {
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
			strings.Join(host.Tags, " "),
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
		if IsCommandProxyHost(host) {
			items = append(items, fmt.Sprintf("%s (via:%s)", name, strings.TrimSpace(host.Via)))
			continue
		}
		items = append(items, fmt.Sprintf("%s (%s:%d)", name, host.IP, host.Port))
	}
	return strings.Join(items, ", ")
}

func validateHost(host Host) error {
	if err := validateHostBackend(host); err != nil {
		return err
	}
	if IsCommandProxyHost(host) {
		if strings.TrimSpace(host.Name) == "" {
			return errors.New("name is required for command_proxy host")
		}
		if strings.TrimSpace(host.Via) == "" {
			return errors.New("via is required for command_proxy host")
		}
		if strings.TrimSpace(host.RunTemplate) == "" && strings.TrimSpace(host.LoginCommand) == "" {
			return errors.New("run_template or login_command is required for command_proxy host")
		}
		if err := ValidateCommandProxyTemplate(host); err != nil {
			return err
		}
		if host.Port < 0 || host.Port > 65535 {
			return fmt.Errorf("invalid ssh port %d", host.Port)
		}
		return nil
	}
	if strings.TrimSpace(host.IP) == "" {
		return errors.New("ip is required")
	}
	if strings.TrimSpace(host.User) == "" && strings.TrimSpace(host.AuthRef) == "" {
		return errors.New("user is required")
	}
	if strings.TrimSpace(host.AuthRef) == "" && !hasPasswordAuth(host) && !hasKeyAuth(host) {
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

func HostBackend(host Host) string {
	backend := strings.TrimSpace(host.Backend)
	if backend == "" {
		return HostBackendSSH
	}
	return backend
}

func IsCommandProxyHost(host Host) bool {
	return HostBackend(host) == HostBackendCommandProxy
}

func validateHostBackend(host Host) error {
	switch HostBackend(host) {
	case HostBackendSSH, HostBackendCommandProxy:
		return nil
	default:
		return fmt.Errorf("invalid backend %q, want ssh or command_proxy", strings.TrimSpace(host.Backend))
	}
}

func ValidateCommandProxyTemplate(host Host) error {
	template := strings.TrimSpace(host.RunTemplate)
	if template != "" && !strings.Contains(template, CommandProxyCmdToken) {
		return fmt.Errorf("run_template for host %q must contain %s", HostLogName(host), CommandProxyCmdToken)
	}
	return nil
}

func NormalizeHostFields(host *Host) {
	host.Name = strings.TrimSpace(host.Name)
	host.IP = strings.TrimSpace(host.IP)
	host.AuthRef = strings.TrimSpace(host.AuthRef)
	host.User = strings.TrimSpace(host.User)
	host.KeyPath = strings.TrimSpace(host.KeyPath)
	host.KeyDataEnc = strings.TrimSpace(host.KeyDataEnc)
	host.KeyPassphraseEnc = strings.TrimSpace(host.KeyPassphraseEnc)
	host.Jump = strings.TrimSpace(host.Jump)
	host.Backend = strings.TrimSpace(host.Backend)
	host.Via = strings.TrimSpace(host.Via)
	host.RunTemplate = strings.TrimSpace(host.RunTemplate)
	host.LoginCommand = strings.TrimSpace(host.LoginCommand)
	host.Remark = strings.TrimSpace(host.Remark)
	host.Group = strings.TrimSpace(host.Group)
	host.Tags = NormalizeTagList(host.Tags)
	host.ConnectTimeout = strings.TrimSpace(host.ConnectTimeout)
	host.RunTimeout = strings.TrimSpace(host.RunTimeout)
	host.RemoteScriptDir = strings.TrimSpace(host.RemoteScriptDir)
	host.HostKeyCheck = strings.TrimSpace(host.HostKeyCheck)
	host.KnownHostsPath = strings.TrimSpace(host.KnownHostsPath)
}

func hasPasswordAuth(host Host) bool {
	return host.Password != "" || host.PasswordEnc != ""
}

func hasKeyAuth(host Host) bool {
	return strings.TrimSpace(host.KeyPath) != "" ||
		host.KeyData != "" ||
		host.KeyDataEnc != ""
}

func NormalizeTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return NormalizeTagList(strings.Split(value, ","))
}

func NormalizeTagList(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	sort.Strings(normalized)
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func HostHasTags(host Host, tags []string) bool {
	tags = NormalizeTagList(tags)
	if len(tags) == 0 {
		return true
	}
	hostTags := NormalizeTagList(host.Tags)
	if len(hostTags) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(hostTags))
	for _, tag := range hostTags {
		seen[tag] = struct{}{}
	}
	for _, tag := range tags {
		if _, ok := seen[tag]; !ok {
			return false
		}
	}
	return true
}

func HostTagsLabel(host Host) string {
	tags := NormalizeTagList(host.Tags)
	if len(tags) == 0 {
		return "-"
	}
	return strings.Join(tags, ",")
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

func SSHConfigPathForImport() (string, error) {
	return sshConfigPath()
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
	if config.Groups == nil {
		config.Groups = map[string]GroupDefaults{}
	}
	normalizeGroups(config.Groups)
	if config.Hosts == nil {
		config.Hosts = []Host{}
	}
	for i := range config.Hosts {
		NormalizeHostFields(&config.Hosts[i])
	}
}

func normalizeConfigForSave(config *Config) {
	config.Version = ConfigVersion
	normalizeConfig(config)
}

func normalizeGroups(groups map[string]GroupDefaults) {
	for name, group := range groups {
		trimmed := strings.TrimSpace(name)
		NormalizeGroupDefaults(&group)
		if trimmed == "" {
			delete(groups, name)
			continue
		}
		if trimmed != name {
			delete(groups, name)
		}
		groups[trimmed] = group
	}
}

func NormalizeGroupDefaults(group *GroupDefaults) {
	group.AuthRef = strings.TrimSpace(group.AuthRef)
	group.User = strings.TrimSpace(group.User)
	group.KeyPath = strings.TrimSpace(group.KeyPath)
	group.Jump = strings.TrimSpace(group.Jump)
	group.ConnectTimeout = strings.TrimSpace(group.ConnectTimeout)
	group.RunTimeout = strings.TrimSpace(group.RunTimeout)
	group.RemoteScriptDir = strings.TrimSpace(group.RemoteScriptDir)
	group.HostKeyCheck = strings.TrimSpace(group.HostKeyCheck)
	group.KnownHostsPath = strings.TrimSpace(group.KnownHostsPath)
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

func ConfigPathSource() string {
	if strings.TrimSpace(os.Getenv(ConfigEnvKey)) != "" {
		return ConfigEnvKey
	}
	if strings.TrimSpace(os.Getenv(ConfigDirEnvKey)) != "" {
		return ConfigDirEnvKey
	}
	return "default"
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
	return path, nil, nil
}

func ReadConfigFile() (string, []byte, error) {
	return readConfigFile()
}

func SetUserHomeDirForTest(fn func() (string, error)) func() {
	old := userHomeDir
	userHomeDir = fn
	return func() { userHomeDir = old }
}
