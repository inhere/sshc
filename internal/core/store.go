package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultSSHPort = 22
	ConfigEnvKey   = "SSHC_CONFIG"
)

var userHomeDir = os.UserHomeDir

type Host struct {
	Name     string `json:"name"`
	IP       string `json:"ip"`
	User     string `json:"user"`
	Password string `json:"password"`
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
	for _, host := range s.Hosts {
		if host.Name == target || host.IP == target {
			return host, true
		}
	}
	return Host{}, false
}

func validateHost(host Host) error {
	if strings.TrimSpace(host.IP) == "" {
		return errors.New("ip is required")
	}
	if strings.TrimSpace(host.User) == "" {
		return errors.New("user is required")
	}
	if host.Password == "" {
		return errors.New("password is required")
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
