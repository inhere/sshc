package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inhere/sshc/internal/core"
)

func TestCfgPathCommand(t *testing.T) {
	path := withTempConfig(t)
	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))

	if err := app.RunWithArgs([]string{"cfg", "path"}); err != nil {
		t.Fatalf("cfg path: %v", err)
	}
	if !strings.Contains(out.String(), path) || !strings.Contains(out.String(), "source=SSHC_CONFIG") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCfgPathCommandWithConfigDirEnv(t *testing.T) {
	t.Setenv(core.ConfigEnvKey, "")
	dir := filepath.Join(t.TempDir(), "sshc-config")
	t.Setenv(core.ConfigDirEnvKey, dir)
	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))

	if err := app.RunWithArgs([]string{"cfg", "path"}); err != nil {
		t.Fatalf("cfg path: %v", err)
	}
	wantPath := filepath.Join(dir, core.ConfigFileName)
	if !strings.Contains(out.String(), wantPath) || !strings.Contains(out.String(), "source=SSHC_CONFIG_DIR") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCfgShowMasksSecrets(t *testing.T) {
	withTempConfig(t)
	config := &core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", Password: "secret"}},
		Hosts:        []core.Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root", Password: "host-secret"}},
	}
	if err := core.SaveConfig(config); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	if err := app.RunWithArgs([]string{"cfg", "show"}); err != nil {
		t.Fatalf("cfg show: %v", err)
	}
	output := out.String()
	if strings.Contains(output, "secret") || !strings.Contains(output, `"password_enc": "***"`) {
		t.Fatalf("output = %q", output)
	}
}

func TestCfgShowRaw(t *testing.T) {
	path := withTempConfig(t)
	raw := `{"logs_path":"raw/logs","hosts":[]}` + "\n"
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	if err := app.RunWithArgs([]string{"cfg", "show", "--raw"}); err != nil {
		t.Fatalf("cfg show --raw: %v", err)
	}
	if out.String() != raw {
		t.Fatalf("output = %q, want %q", out.String(), raw)
	}
}

func TestCfgSetGetUnsetLogsPath(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))

	if err := app.RunWithArgs([]string{"cfg", "set", "logs_path", "./runtime/logs"}); err != nil {
		t.Fatalf("cfg set: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.LogsPath != "./runtime/logs" {
		t.Fatalf("logs_path = %q", config.LogsPath)
	}

	out.Reset()
	if err := app.RunWithArgs([]string{"cfg", "get", "logs_path"}); err != nil {
		t.Fatalf("cfg get: %v", err)
	}
	if strings.TrimSpace(out.String()) != "./runtime/logs" {
		t.Fatalf("output = %q", out.String())
	}

	if err := app.RunWithArgs([]string{"cfg", "unset", "logs_path"}); err != nil {
		t.Fatalf("cfg unset: %v", err)
	}
	config, err = core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.LogsPath != "" {
		t.Fatalf("logs_path = %q", config.LogsPath)
	}
}

func TestCfgSetGetUnsetDefaults(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))

	cases := []struct {
		key   string
		value string
	}{
		{key: "defaults.user", value: "root"},
		{key: "defaults.port", value: "2222"},
		{key: "defaults.connect_timeout", value: "15s"},
		{key: "defaults.run_timeout", value: "2m"},
		{key: "defaults.remote_script_dir", value: "/var/tmp"},
		{key: "defaults.host_key_check", value: core.HostKeyCheckKnownHosts},
		{key: "defaults.known_hosts_path", value: "~/.ssh/known_hosts"},
	}
	for _, tt := range cases {
		out.Reset()
		if err := app.RunWithArgs([]string{"cfg", "set", tt.key, tt.value}); err != nil {
			t.Fatalf("cfg set %s: %v", tt.key, err)
		}
		out.Reset()
		if err := app.RunWithArgs([]string{"cfg", "get", tt.key}); err != nil {
			t.Fatalf("cfg get %s: %v", tt.key, err)
		}
		if strings.TrimSpace(out.String()) != tt.value {
			t.Fatalf("cfg get %s = %q, want %q", tt.key, out.String(), tt.value)
		}
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Defaults.User != "root" || config.Defaults.Port != 2222 || config.Defaults.HostKeyCheck != core.HostKeyCheckKnownHosts {
		t.Fatalf("defaults = %+v", config.Defaults)
	}

	for _, tt := range cases {
		if err := app.RunWithArgs([]string{"cfg", "unset", tt.key}); err != nil {
			t.Fatalf("cfg unset %s: %v", tt.key, err)
		}
	}
	config, err = core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Defaults != (core.Defaults{}) {
		t.Fatalf("defaults = %+v", config.Defaults)
	}
}

func TestCfgSetDefaultsValidation(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()

	if err := app.RunWithArgs([]string{"cfg", "set", "defaults.port", "70000"}); err == nil || !strings.Contains(err.Error(), "invalid defaults.port") {
		t.Fatalf("port err = %v", err)
	}
	if err := app.RunWithArgs([]string{"cfg", "set", "defaults.host_key_check", "bad"}); err == nil || !strings.Contains(err.Error(), "invalid defaults.host_key_check") {
		t.Fatalf("host key err = %v", err)
	}
	if err := app.RunWithArgs([]string{"cfg", "get", "defaults.password"}); err == nil || !strings.Contains(err.Error(), "unsupported config key") {
		t.Fatalf("unsupported err = %v", err)
	}
}

func TestCfgDoctorReturnsErrorForInvalidConfig(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{
		{Name: "devhost", IP: "10.0.0.8"},
		{Name: "devhost", IP: "10.0.0.9"},
	}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	err := app.RunWithArgs([]string{"cfg", "doctor"})
	if err == nil || !strings.Contains(err.Error(), "doctor found errors") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out.String(), "duplicate host name") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCfgExportWritesEncryptedFile(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", Password: "secret"}},
		Hosts:        []core.Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root"}},
	}); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "sshc-export.enc")
	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))

	if err := app.RunWithArgs([]string{"cfg", "export", "-o", output}); err != nil {
		t.Fatalf("cfg export: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "devhost") {
		t.Fatalf("export leaked plaintext: %s", data)
	}
	if !strings.Contains(out.String(), "exported config to") || !strings.Contains(out.String(), "export key: sshc-v1:") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCfgExportPrintsExportKey(t *testing.T) {
	withTempConfig(t)
	output := filepath.Join(t.TempDir(), "sshc-export.enc")
	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))

	if err := app.RunWithArgs([]string{"cfg", "export", "-o", output}); err != nil {
		t.Fatalf("cfg export: %v", err)
	}
	if !strings.Contains(out.String(), "export key: sshc-v1:") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCfgExportRejectsExistingFileWithoutForce(t *testing.T) {
	withTempConfig(t)
	output := filepath.Join(t.TempDir(), "sshc-export.enc")
	if err := os.WriteFile(output, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	app := newTestApp()
	err := app.RunWithArgs([]string{"cfg", "export", "-o", output})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing" {
		t.Fatalf("file was overwritten: %s", data)
	}
}

func TestCfgExportForceOverwritesFile(t *testing.T) {
	withTempConfig(t)
	output := filepath.Join(t.TempDir(), "sshc-export.enc")
	if err := os.WriteFile(output, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	app := newTestApp()
	if err := app.RunWithArgs([]string{"cfg", "export", "-o", output, "--force"}); err != nil {
		t.Fatalf("cfg export --force: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "existing" || !strings.Contains(string(data), `"payload"`) {
		t.Fatalf("file not overwritten with export: %s", data)
	}
}

func TestCfgExportRequiresOutput(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	err := app.RunWithArgs([]string{"cfg", "export"})
	if err == nil || !strings.Contains(err.Error(), "--output is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestCfgImportMergeAddsEntries(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "current", User: "root", KeyPath: "~/.ssh/current"}},
		Hosts:        []core.Host{{Name: "current", IP: "10.0.0.8", AuthRef: "current"}},
	}); err != nil {
		t.Fatal(err)
	}
	file, key := writeConfigExportForTest(t, core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "imported", User: "ops", KeyPath: "~/.ssh/imported"}},
		Hosts:        []core.Host{{Name: "imported", IP: "10.0.0.9", AuthRef: "imported"}},
	})
	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))

	if err := app.RunWithArgs([]string{"cfg", "import", "-f", file, "--key", key}); err != nil {
		t.Fatalf("cfg import: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AuthProfiles) != 2 || len(config.Hosts) != 2 {
		t.Fatalf("config = %+v", config)
	}
	if !strings.Contains(out.String(), "hosts_added=1") || !strings.Contains(out.String(), "auth_added=1") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCfgImportMergeRejectsConflicts(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/current"}},
		Hosts:        []core.Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root"}},
	}); err != nil {
		t.Fatal(err)
	}
	file, key := writeConfigExportForTest(t, core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "ops", KeyPath: "~/.ssh/imported"}},
		Hosts:        []core.Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root"}},
	})
	app := newTestApp()
	err := app.RunWithArgs([]string{"cfg", "import", "-f", file, "--key", key})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestCfgImportOverwriteUpdatesEntries(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/current"}},
		Hosts:        []core.Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root", Remark: "old"}},
	}); err != nil {
		t.Fatal(err)
	}
	file, key := writeConfigExportForTest(t, core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "ops", KeyPath: "~/.ssh/imported"}},
		Hosts:        []core.Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root", Remark: "new"}},
	})
	app := newTestApp()
	if err := app.RunWithArgs([]string{"cfg", "import", "-f", file, "--key", key, "--overwrite"}); err != nil {
		t.Fatalf("cfg import overwrite: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.AuthProfiles[0].User != "ops" || config.Hosts[0].Remark != "new" {
		t.Fatalf("config = %+v", config)
	}
}

func TestCfgImportReplaceConfig(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{Name: "old", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/old"}}}); err != nil {
		t.Fatal(err)
	}
	file, key := writeConfigExportForTest(t, core.Config{Hosts: []core.Host{{Name: "new", IP: "10.0.0.9", User: "root", KeyPath: "~/.ssh/new"}}})
	app := newTestApp()
	if err := app.RunWithArgs([]string{"cfg", "import", "-f", file, "--key", key, "--replace"}); err != nil {
		t.Fatalf("cfg import replace: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Hosts) != 1 || config.Hosts[0].Name != "new" {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestCfgImportRequiresFileAndKey(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	if err := app.RunWithArgs([]string{"cfg", "import"}); err == nil || !strings.Contains(err.Error(), "--file is required") {
		t.Fatalf("file err = %v", err)
	}
	if err := app.RunWithArgs([]string{"cfg", "import", "-f", "missing.enc"}); err == nil || !strings.Contains(err.Error(), "--key is required") {
		t.Fatalf("key err = %v", err)
	}
}

func TestCfgImportRejectsMultipleStrategies(t *testing.T) {
	withTempConfig(t)
	file, key := writeConfigExportForTest(t, core.Config{})
	app := newTestApp()
	err := app.RunWithArgs([]string{"cfg", "import", "-f", file, "--key", key, "--merge", "--overwrite"})
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("err = %v", err)
	}
}

func TestCfgImportBacksUpExistingConfig(t *testing.T) {
	path := withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{Name: "old", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/old"}}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, key := writeConfigExportForTest(t, core.Config{Hosts: []core.Host{{Name: "new", IP: "10.0.0.9", User: "root", KeyPath: "~/.ssh/new"}}})
	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))

	if err := app.RunWithArgs([]string{"cfg", "import", "-f", file, "--key", key, "--replace"}); err != nil {
		t.Fatalf("cfg import replace: %v", err)
	}
	if !strings.Contains(out.String(), "backup:") || strings.Contains(out.String(), "backup: none") {
		t.Fatalf("output = %q", out.String())
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "backups", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("backups = %+v", backups)
	}
	data, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(before) {
		t.Fatalf("backup mismatch:\n%s\n%s", data, before)
	}
}

func TestCfgImportReencryptsPasswordsWithLocalKey(t *testing.T) {
	path := withTempConfig(t)
	file, key := writeConfigExportForTest(t, core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", Password: "secret"}},
		Hosts:        []core.Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root", Password: "host-secret"}},
	})
	app := newTestApp()
	if err := app.RunWithArgs([]string{"cfg", "import", "-f", file, "--key", key}); err != nil {
		t.Fatalf("cfg import password: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, `"password": "secret"`) || strings.Contains(content, `"password": "host-secret"`) {
		t.Fatalf("config contains plaintext password: %s", content)
	}
	if !strings.Contains(content, `"password_enc": "v1:`) {
		t.Fatalf("config missing password_enc: %s", content)
	}
}

func writeConfigExportForTest(t *testing.T, config core.Config) (string, string) {
	t.Helper()
	key, err := core.GenerateExportKey()
	if err != nil {
		t.Fatal(err)
	}
	data, err := core.EncryptConfigExport(config, key, time.Date(2026, 7, 5, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sshc-export.enc")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path, key
}
