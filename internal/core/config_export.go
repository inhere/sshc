package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/hkdf"
)

const (
	ConfigExportVersion = 1
	ConfigExportApp     = "sshc"

	configExportCipher = "AES-256-GCM"
	configExportKDF    = "HKDF-SHA256"
	configExportPrefix = "sshc-v1:"
	configExportKeyLen = 32
	configExportInfo   = "sshc config export v1"
)

type ConfigExportFile struct {
	Version   int    `json:"version"`
	Cipher    string `json:"cipher"`
	KDF       string `json:"kdf"`
	CreatedAt string `json:"created_at"`
	Nonce     string `json:"nonce"`
	Payload   string `json:"payload"`
}

type ConfigExportPayload struct {
	Version       int    `json:"version"`
	App           string `json:"app"`
	ConfigVersion int    `json:"config_version"`
	ExportedAt    string `json:"exported_at"`
	Config        Config `json:"config"`
}

type ImportStrategy string

const (
	ImportMerge     ImportStrategy = "merge"
	ImportOverwrite ImportStrategy = "overwrite"
	ImportReplace   ImportStrategy = "replace"
)

type ImportResult struct {
	BackupPath    string
	HostsAdded    int
	HostsUpdated  int
	GroupsAdded   int
	GroupsUpdated int
	AuthAdded     int
	AuthUpdated   int
}

func GenerateExportKey() (string, error) {
	key := make([]byte, configExportKeyLen)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", err
	}
	return configExportPrefix + base64.RawURLEncoding.EncodeToString(key), nil
}

func EncryptConfigExport(config Config, key string, now time.Time) ([]byte, error) {
	aead, err := configExportAEAD(key)
	if err != nil {
		return nil, err
	}
	normalizeConfig(&config)
	payload := ConfigExportPayload{
		Version:       ConfigExportVersion,
		App:           ConfigExportApp,
		ConfigVersion: ConfigVersion,
		ExportedAt:    formatExportTime(now),
		Config:        config,
	}
	plain, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	encrypted := aead.Seal(nil, nonce, plain, nil)
	file := ConfigExportFile{
		Version:   ConfigExportVersion,
		Cipher:    configExportCipher,
		KDF:       configExportKDF,
		CreatedAt: formatExportTime(now),
		Nonce:     base64.RawStdEncoding.EncodeToString(nonce),
		Payload:   base64.RawStdEncoding.EncodeToString(encrypted),
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func DecryptConfigExport(data []byte, key string) (Config, error) {
	var file ConfigExportFile
	if err := json.Unmarshal(data, &file); err != nil {
		return Config{}, err
	}
	if file.Version != ConfigExportVersion {
		return Config{}, fmt.Errorf("unsupported config export version %d", file.Version)
	}
	if file.Cipher != configExportCipher {
		return Config{}, fmt.Errorf("unsupported config export cipher %q", file.Cipher)
	}
	if file.KDF != configExportKDF {
		return Config{}, fmt.Errorf("unsupported config export kdf %q", file.KDF)
	}
	aead, err := configExportAEAD(key)
	if err != nil {
		return Config{}, err
	}
	nonce, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(file.Nonce))
	if err != nil {
		return Config{}, fmt.Errorf("decode config export nonce: %w", err)
	}
	encrypted, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(file.Payload))
	if err != nil {
		return Config{}, fmt.Errorf("decode config export payload: %w", err)
	}
	plain, err := aead.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return Config{}, fmt.Errorf("decrypt config export: %w", err)
	}
	var payload ConfigExportPayload
	if err := json.Unmarshal(plain, &payload); err != nil {
		return Config{}, err
	}
	if payload.Version != ConfigExportVersion {
		return Config{}, fmt.Errorf("unsupported config export payload version %d", payload.Version)
	}
	if payload.App != ConfigExportApp {
		return Config{}, fmt.Errorf("unsupported config export app %q", payload.App)
	}
	config := payload.Config
	normalizeConfig(&config)
	return config, nil
}

func MergeImportedConfig(current, imported Config, strategy ImportStrategy) (Config, ImportResult, error) {
	normalizeConfig(&current)
	normalizeConfig(&imported)
	switch strategy {
	case "", ImportMerge:
		return mergeImportedConfig(current, imported, false)
	case ImportOverwrite:
		return mergeImportedConfig(current, imported, true)
	case ImportReplace:
		result := ImportResult{
			HostsAdded:  len(imported.Hosts),
			GroupsAdded: len(imported.Groups),
			AuthAdded:   len(imported.AuthProfiles),
		}
		normalizeConfig(&imported)
		return imported, result, nil
	default:
		return Config{}, ImportResult{}, fmt.Errorf("unsupported import strategy %q", strategy)
	}
}

func BackupConfigFile(now time.Time) (string, error) {
	path, data, err := ReadConfigFile()
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", nil
	}
	storePath, err := StorePath()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(filepath.Dir(storePath), "backups")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	backupPath := filepath.Join(dir, fmt.Sprintf("%s.%s.json", strings.TrimSuffix(filepath.Base(storePath), filepath.Ext(storePath)), now.Format("20060102-150405")))
	if filepath.Clean(path) == filepath.Clean(backupPath) {
		return "", errors.New("backup path resolves to config path")
	}
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", err
	}
	return backupPath, nil
}

func mergeImportedConfig(current, imported Config, overwrite bool) (Config, ImportResult, error) {
	result := ImportResult{}
	merged := current
	if overwrite {
		if strings.TrimSpace(imported.LogsPath) != "" {
			merged.LogsPath = strings.TrimSpace(imported.LogsPath)
		}
		merged.Defaults = overwriteDefaults(merged.Defaults, imported.Defaults)
	} else {
		if strings.TrimSpace(merged.LogsPath) == "" {
			merged.LogsPath = strings.TrimSpace(imported.LogsPath)
		}
		merged.Defaults = mergeDefaults(merged.Defaults, imported.Defaults)
	}
	if merged.Groups == nil {
		merged.Groups = map[string]GroupDefaults{}
	}
	for name, group := range imported.Groups {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		NormalizeGroupDefaults(&group)
		if _, ok := merged.Groups[name]; ok {
			if !overwrite {
				return Config{}, ImportResult{}, fmt.Errorf("group %q already exists", name)
			}
			merged.Groups[name] = group
			result.GroupsUpdated++
			continue
		}
		merged.Groups[name] = group
		result.GroupsAdded++
	}

	for _, profile := range imported.AuthProfiles {
		idx := findAuthProfileIndex(merged.AuthProfiles, profile.Name)
		if idx >= 0 {
			if !overwrite {
				return Config{}, ImportResult{}, fmt.Errorf("auth profile %q already exists", profile.Name)
			}
			merged.AuthProfiles[idx] = profile
			result.AuthUpdated++
			continue
		}
		merged.AuthProfiles = append(merged.AuthProfiles, profile)
		result.AuthAdded++
	}

	for _, host := range imported.Hosts {
		nameIdx := findConfigHostByName(merged.Hosts, host.Name)
		ipIdx := findConfigHostByIP(merged.Hosts, host.IP)
		idx := mergeImportIndex(nameIdx, ipIdx)
		if idx == -2 {
			return Config{}, ImportResult{}, fmt.Errorf("host %q conflicts with different existing hosts by name and ip", HostLogName(host))
		}
		if idx >= 0 {
			if !overwrite {
				if nameIdx >= 0 {
					return Config{}, ImportResult{}, fmt.Errorf("host %q already exists", host.Name)
				}
				return Config{}, ImportResult{}, fmt.Errorf("host ip %q already exists", host.IP)
			}
			merged.Hosts[idx] = host
			result.HostsUpdated++
			continue
		}
		merged.Hosts = append(merged.Hosts, host)
		result.HostsAdded++
	}
	normalizeConfig(&merged)
	return merged, result, nil
}

func configExportAEAD(key string) (cipher.AEAD, error) {
	raw, err := parseExportKey(key)
	if err != nil {
		return nil, err
	}
	derived := make([]byte, configExportKeyLen)
	reader := hkdf.New(sha256.New, raw, nil, []byte(configExportInfo))
	if _, err := io.ReadFull(reader, derived); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(derived)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func parseExportKey(key string) ([]byte, error) {
	key = strings.TrimSpace(key)
	if !strings.HasPrefix(key, configExportPrefix) {
		return nil, fmt.Errorf("invalid export key format, want %s...", configExportPrefix)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(key, configExportPrefix))
	if err != nil {
		return nil, fmt.Errorf("decode export key: %w", err)
	}
	if len(raw) != configExportKeyLen {
		return nil, fmt.Errorf("invalid export key length %d", len(raw))
	}
	return raw, nil
}

func mergeDefaults(current, imported Defaults) Defaults {
	if strings.TrimSpace(current.User) == "" {
		current.User = strings.TrimSpace(imported.User)
	}
	if current.Port == 0 {
		current.Port = imported.Port
	}
	if strings.TrimSpace(current.ConnectTimeout) == "" {
		current.ConnectTimeout = strings.TrimSpace(imported.ConnectTimeout)
	}
	if strings.TrimSpace(current.RunTimeout) == "" {
		current.RunTimeout = strings.TrimSpace(imported.RunTimeout)
	}
	if strings.TrimSpace(current.RemoteScriptDir) == "" {
		current.RemoteScriptDir = strings.TrimSpace(imported.RemoteScriptDir)
	}
	if strings.TrimSpace(current.HostKeyCheck) == "" {
		current.HostKeyCheck = strings.TrimSpace(imported.HostKeyCheck)
	}
	if strings.TrimSpace(current.KnownHostsPath) == "" {
		current.KnownHostsPath = strings.TrimSpace(imported.KnownHostsPath)
	}
	return current
}

func overwriteDefaults(current, imported Defaults) Defaults {
	if strings.TrimSpace(imported.User) != "" {
		current.User = strings.TrimSpace(imported.User)
	}
	if imported.Port != 0 {
		current.Port = imported.Port
	}
	if strings.TrimSpace(imported.ConnectTimeout) != "" {
		current.ConnectTimeout = strings.TrimSpace(imported.ConnectTimeout)
	}
	if strings.TrimSpace(imported.RunTimeout) != "" {
		current.RunTimeout = strings.TrimSpace(imported.RunTimeout)
	}
	if strings.TrimSpace(imported.RemoteScriptDir) != "" {
		current.RemoteScriptDir = strings.TrimSpace(imported.RemoteScriptDir)
	}
	if strings.TrimSpace(imported.HostKeyCheck) != "" {
		current.HostKeyCheck = strings.TrimSpace(imported.HostKeyCheck)
	}
	if strings.TrimSpace(imported.KnownHostsPath) != "" {
		current.KnownHostsPath = strings.TrimSpace(imported.KnownHostsPath)
	}
	return current
}

func findAuthProfileIndex(profiles []AuthProfile, name string) int {
	name = strings.TrimSpace(name)
	if name == "" {
		return -1
	}
	for i, profile := range profiles {
		if strings.TrimSpace(profile.Name) == name {
			return i
		}
	}
	return -1
}

func findConfigHostByName(hosts []Host, name string) int {
	name = strings.TrimSpace(name)
	if name == "" {
		return -1
	}
	for i, host := range hosts {
		if strings.TrimSpace(host.Name) == name {
			return i
		}
	}
	return -1
}

func findConfigHostByIP(hosts []Host, ip string) int {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return -1
	}
	for i, host := range hosts {
		if strings.TrimSpace(host.IP) == ip {
			return i
		}
	}
	return -1
}

func mergeImportIndex(a, b int) int {
	switch {
	case a >= 0 && b >= 0 && a != b:
		return -2
	case a >= 0:
		return a
	default:
		return b
	}
}

func formatExportTime(t time.Time) string {
	return t.Format("2006-01-02T15:04:05.000")
}
