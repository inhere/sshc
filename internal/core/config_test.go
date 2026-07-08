package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreUpsertReplacesByNameOrIP(t *testing.T) {
	store := &Store{}
	if err := store.Upsert(Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "one", Port: 22}); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(Host{Name: "devhost", IP: "10.0.0.9", User: "ops", Password: "two", Port: 22}); err != nil {
		t.Fatal(err)
	}
	if len(store.Hosts) != 1 {
		t.Fatalf("hosts len = %d, want 1", len(store.Hosts))
	}
	if store.Hosts[0].IP != "10.0.0.9" || store.Hosts[0].User != "ops" {
		t.Fatalf("unexpected replacement: %+v", store.Hosts[0])
	}
}

func TestValidateHostAllowsKeyPathWithoutPassword(t *testing.T) {
	err := validateHost(Host{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22})
	if err != nil {
		t.Fatalf("validateHost with key path: %v", err)
	}
	err = validateHost(Host{Name: "devhost", IP: "10.0.0.8", User: "root", Port: 22})
	if err == nil || !strings.Contains(err.Error(), "password or key_path") {
		t.Fatalf("err = %v", err)
	}
}

func TestSaveStoreEncryptsPasswordAndLoadDecrypts(t *testing.T) {
	path := withTempConfig(t)
	storeToSave := &Store{Hosts: []Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     22,
	}}}
	if err := SaveStore(storeToSave); err != nil {
		t.Fatal(err)
	}
	if storeToSave.Hosts[0].Password != "secret" {
		t.Fatalf("SaveStore mutated source password: %+v", storeToSave.Hosts[0])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, `"password":"secret"`) || strings.Contains(content, `"password": "secret"`) {
		t.Fatalf("stored config contains plaintext password: %s", content)
	}
	if !strings.Contains(content, `"password_enc": "v1:`) {
		t.Fatalf("stored config does not contain encrypted password: %s", content)
	}

	store, err := LoadStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Hosts) != 1 || store.Hosts[0].Password != "secret" {
		t.Fatalf("loaded store = %+v", store.Hosts)
	}
	keyPath, err := PasswordKeyPath()
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(keyPath); err != nil || info.IsDir() {
		t.Fatalf("password key file not created: info=%v err=%v", info, err)
	}
}

func TestLoadStoreSupportsLegacyPlaintextPassword(t *testing.T) {
	path := withTempConfig(t)
	data := []byte(`{"hosts":[{"name":"devhost","ip":"10.0.0.8","user":"root","password":"secret","port":22}]}` + "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	store, err := LoadStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Hosts) != 1 || store.Hosts[0].Password != "secret" {
		t.Fatalf("loaded store = %+v", store.Hosts)
	}
}

func TestLoadConfigLegacyStoreShape(t *testing.T) {
	path := withTempConfig(t)
	data := []byte(`{"logs_path":"runtime/logs","hosts":[{"name":"devhost","ip":"10.0.0.8","user":"root","password":"secret","port":22}]}` + "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != ConfigVersion || config.LogsPath != "runtime/logs" {
		t.Fatalf("config = %+v", config)
	}
	if len(config.AuthProfiles) != 0 {
		t.Fatalf("auth profiles = %+v", config.AuthProfiles)
	}
	if len(config.Hosts) != 1 || config.Hosts[0].Password != "secret" {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestLoadConfigVersionedShape(t *testing.T) {
	path := withTempConfig(t)
	data := []byte(`{
  "version": 1,
  "logs_path": "logs",
  "defaults": {"user":"root","port":22},
  "auth_profiles": [{"name":"dev-root","user":"root","password":"secret"}],
  "hosts": [{"name":"devhost","ip":"10.0.0.8","auth_ref":"dev-root","port":22}]
}` + "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != ConfigVersion || config.Defaults.User != "root" {
		t.Fatalf("config = %+v", config)
	}
	if len(config.AuthProfiles) != 1 || config.AuthProfiles[0].Name != "dev-root" || config.AuthProfiles[0].Password != "secret" {
		t.Fatalf("auth profiles = %+v", config.AuthProfiles)
	}
	if len(config.Hosts) != 1 || config.Hosts[0].AuthRef != "dev-root" {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestLoadConfigWithEnvPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom-config.json")
	t.Setenv(ConfigEnvKey, path)
	if err := os.WriteFile(path, []byte(`{"logs_path":"custom/logs","hosts":[]}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.LogsPath != "custom/logs" {
		t.Fatalf("logs_path = %q", config.LogsPath)
	}
}

func TestSaveConfigWritesVersionOne(t *testing.T) {
	path := withTempConfig(t)
	config := &Config{
		LogsPath:     "runtime/logs",
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}},
		Hosts:        []Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root"}},
	}
	if err := SaveConfig(config); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Version != ConfigVersion {
		t.Fatalf("version = %d, want %d; json=%s", saved.Version, ConfigVersion, string(data))
	}
	if config.Version != 0 {
		t.Fatalf("SaveConfig mutated source version: %+v", config)
	}
}

func TestLoadStoreStillDecryptsHostPasswordEnc(t *testing.T) {
	path := withTempConfig(t)
	encrypted, err := EncryptPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"version":1,"hosts":[{"name":"devhost","ip":"10.0.0.8","user":"root","password_enc":"` + encrypted + `","port":22}]}` + "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	store, err := LoadStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Hosts) != 1 || store.Hosts[0].Password != "secret" {
		t.Fatalf("store = %+v", store)
	}
}

func TestLoadConfigSettingsReadsLogsPath(t *testing.T) {
	path := withTempConfig(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"logs_path":"runtime/logs","hosts":[]}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	settings, err := LoadConfigSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.LogsPath != "runtime/logs" {
		t.Fatalf("logs_path = %q", settings.LogsPath)
	}
}

func TestSaveConfigEncryptsAuthProfilePassword(t *testing.T) {
	path := withTempConfig(t)
	config := &Config{
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root", Password: "secret"}},
		Hosts:        []Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root"}},
	}
	if err := SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	if config.AuthProfiles[0].Password != "secret" {
		t.Fatalf("SaveConfig mutated source auth profile: %+v", config.AuthProfiles[0])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, `"password":"secret"`) || strings.Contains(content, `"password": "secret"`) {
		t.Fatalf("stored config contains plaintext password: %s", content)
	}
	if !strings.Contains(content, `"password_enc": "v1:`) {
		t.Fatalf("stored config does not contain encrypted profile password: %s", content)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.AuthProfiles) != 1 || loaded.AuthProfiles[0].Password != "secret" {
		t.Fatalf("loaded auth profiles = %+v", loaded.AuthProfiles)
	}
}

func TestDecryptConfigPasswordMissingKey(t *testing.T) {
	withTempConfig(t)
	config := &Config{AuthProfiles: []AuthProfile{{Name: "dev-root", PasswordEnc: "v1:AAAA"}}}
	err := decryptConfigPasswords(config)
	if err == nil || !strings.Contains(err.Error(), "password key file not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestMaskConfigHidesSensitiveFields(t *testing.T) {
	config := Config{
		AuthProfiles: []AuthProfile{{Name: "dev-root", Password: "secret", PasswordEnc: "v1:secret"}},
		Hosts:        []Host{{Name: "devhost", IP: "10.0.0.8", Password: "host-secret", PasswordEnc: "v1:host-secret"}},
	}

	masked := MaskConfig(config)
	if masked.AuthProfiles[0].Password != "" || masked.AuthProfiles[0].PasswordEnc != MaskedSecret {
		t.Fatalf("masked profile = %+v", masked.AuthProfiles[0])
	}
	if masked.Hosts[0].Password != "" || masked.Hosts[0].PasswordEnc != MaskedSecret {
		t.Fatalf("masked host = %+v", masked.Hosts[0])
	}
	if config.AuthProfiles[0].Password != "secret" || config.Hosts[0].Password != "host-secret" {
		t.Fatalf("MaskConfig mutated input: %+v", config)
	}
}

func TestAuthLabel(t *testing.T) {
	tests := []struct {
		host Host
		want string
	}{
		{host: Host{AuthRef: "dev-root", KeyPath: "~/.ssh/id_rsa"}, want: "auth:dev-root"},
		{host: Host{KeyPath: "~/.ssh/id_rsa", Password: "secret"}, want: "key+password"},
		{host: Host{KeyPath: "~/.ssh/id_rsa"}, want: "key"},
		{host: Host{PasswordEnc: "v1:secret"}, want: "password"},
		{host: Host{}, want: "-"},
	}
	for _, tt := range tests {
		if got := AuthLabel(tt.host); got != tt.want {
			t.Fatalf("AuthLabel(%+v) = %q, want %q", tt.host, got, tt.want)
		}
	}
}

func TestDoctorReportsDuplicateHosts(t *testing.T) {
	issues := CheckConfig(Config{Hosts: []Host{
		{Name: "devhost", IP: "10.0.0.8"},
		{Name: "devhost", IP: "10.0.0.8"},
	}})
	if !HasDoctorErrors(issues) || !doctorMessagesContain(issues, "duplicate host name") || !doctorMessagesContain(issues, "duplicate host ip") {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestDoctorReportsMissingAuthRef(t *testing.T) {
	issues := CheckConfig(Config{Hosts: []Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "missing"}}})
	if !HasDoctorErrors(issues) || !doctorMessagesContain(issues, `missing auth profile "missing"`) {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestDoctorReportsInvalidPortAndHostKeyCheck(t *testing.T) {
	issues := CheckConfig(Config{
		Defaults: Defaults{HostKeyCheck: "bad"},
		Hosts:    []Host{{Name: "devhost", IP: "10.0.0.8", Port: 70000, HostKeyCheck: "bad"}},
	})
	if !HasDoctorErrors(issues) || !doctorMessagesContain(issues, "invalid port") || !doctorMessagesContain(issues, "invalid host_key_check") {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestDoctorReportsCommandProxyErrors(t *testing.T) {
	issues := CheckConfig(Config{Hosts: []Host{{
		Name:        "lxc-app",
		Backend:     HostBackendCommandProxy,
		Via:         "missing",
		RunTemplate: "pct exec 101 -- hostname",
	}}})
	if !HasDoctorErrors(issues) ||
		!doctorMessagesContain(issues, "references missing via host") ||
		!doctorMessagesContain(issues, "must contain {{cmd}}") {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestDoctorAcceptsCommandProxyHost(t *testing.T) {
	issues := CheckConfig(Config{Hosts: []Host{
		{Name: "pve-host", IP: "192.168.1.20", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22},
		{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host", RunTemplate: "pct exec 101 -- sh -lc {{cmd}}", LoginCommand: "pct enter 101"},
	}})
	if HasDoctorErrors(issues) {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestValidateHostAllowsCommandProxyWithoutIP(t *testing.T) {
	err := validateHost(Host{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host", RunTemplate: "pct exec 101 -- sh -lc {{cmd}}"})
	if err != nil {
		t.Fatalf("validate command_proxy: %v", err)
	}
}

func doctorMessagesContain(issues []DoctorIssue, want string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, want) {
			return true
		}
	}
	return false
}

func TestStorePathDefaultsToDotConfig(t *testing.T) {
	t.Setenv(ConfigEnvKey, "")
	t.Setenv(ConfigDirEnvKey, "")

	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(SetUserHomeDirForTest(func() (string, error) { return home, nil }))

	path, err := StorePath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "sshc", ConfigFileName)
	if path != want {
		t.Fatalf("store path = %q, want %q", path, want)
	}
}

func TestStorePathUsesConfigDirEnv(t *testing.T) {
	t.Setenv(ConfigEnvKey, "")
	dir := filepath.Join(t.TempDir(), "sshc-config")
	t.Setenv(ConfigDirEnvKey, dir)

	path, err := StorePath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, ConfigFileName); path != want {
		t.Fatalf("store path = %q, want %q", path, want)
	}
	keyPath, err := PasswordKeyPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, passwordKeyFile); keyPath != want {
		t.Fatalf("key path = %q, want %q", keyPath, want)
	}
	if source := ConfigPathSource(); source != ConfigDirEnvKey {
		t.Fatalf("source = %q, want %s", source, ConfigDirEnvKey)
	}
}

func TestConfigEnvOverridesConfigDirEnv(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sshc-config")
	path := filepath.Join(t.TempDir(), "custom.json")
	t.Setenv(ConfigDirEnvKey, dir)
	t.Setenv(ConfigEnvKey, path)

	got, err := StorePath()
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("store path = %q, want %q", got, path)
	}
	if source := ConfigPathSource(); source != ConfigEnvKey {
		t.Fatalf("source = %q, want %s", source, ConfigEnvKey)
	}
}

func TestLoadConfigIgnoresHostsJSONInConfigDir(t *testing.T) {
	t.Setenv(ConfigEnvKey, "")
	dir := filepath.Join(t.TempDir(), "sshc-config")
	t.Setenv(ConfigDirEnvKey, dir)

	hostsPath := filepath.Join(dir, "hosts.json")
	if err := os.MkdirAll(filepath.Dir(hostsPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostsPath, []byte(`{"hosts":[{"name":"devhost","ip":"10.0.0.8","user":"root","password":"secret","port":22}]}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != ConfigVersion || len(config.Hosts) != 0 {
		t.Fatalf("config = %+v", config)
	}
}

func TestLoadStoreIgnoresHostsJSON(t *testing.T) {
	t.Setenv(ConfigEnvKey, "")
	t.Setenv(ConfigDirEnvKey, "")
	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(SetUserHomeDirForTest(func() (string, error) { return home, nil }))

	hostsPath := filepath.Join(home, ".config", "sshc", "hosts.json")
	if err := os.MkdirAll(filepath.Dir(hostsPath), 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"hosts":[{"name":"devhost","ip":"10.0.0.8","user":"root","password":"secret","port":22}]}` + "\n")
	if err := os.WriteFile(hostsPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	store, err := LoadStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Hosts) != 0 {
		t.Fatalf("loaded store = %+v", store)
	}
}
