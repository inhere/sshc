package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateExportKey(t *testing.T) {
	key, err := GenerateExportKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, configExportPrefix) {
		t.Fatalf("key = %q", key)
	}
	raw, err := parseExportKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != configExportKeyLen {
		t.Fatalf("raw len = %d", len(raw))
	}
}

func TestEncryptDecryptConfigExport(t *testing.T) {
	key, err := GenerateExportKey()
	if err != nil {
		t.Fatal(err)
	}
	config := Config{
		LogsPath: "logs",
		AuthProfiles: []AuthProfile{{
			Name:     "dev-root",
			User:     "root",
			Password: "secret",
		}},
		Hosts: []Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root", Password: "host-secret"}},
	}
	data, err := EncryptConfigExport(config, key, time.Date(2026, 7, 5, 10, 0, 0, 123456789, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "secret") || strings.Contains(content, "devhost") {
		t.Fatalf("export file leaked plaintext: %s", content)
	}
	var file ConfigExportFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if file.CreatedAt != "2026-07-05T10:00:00.123" {
		t.Fatalf("created_at = %q", file.CreatedAt)
	}

	got, err := DecryptConfigExport(data, key)
	if err != nil {
		t.Fatal(err)
	}
	if got.LogsPath != "logs" || len(got.AuthProfiles) != 1 || got.AuthProfiles[0].Password != "secret" {
		t.Fatalf("config = %+v", got)
	}
	if len(got.Hosts) != 1 || got.Hosts[0].Password != "host-secret" {
		t.Fatalf("hosts = %+v", got.Hosts)
	}
}

func TestDecryptConfigExportRejectsWrongKey(t *testing.T) {
	key, err := GenerateExportKey()
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := GenerateExportKey()
	if err != nil {
		t.Fatal(err)
	}
	data, err := EncryptConfigExport(Config{}, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptConfigExport(data, wrong); err == nil {
		t.Fatal("expected wrong key error")
	}
}

func TestDecryptConfigExportRejectsBadVersion(t *testing.T) {
	key, err := GenerateExportKey()
	if err != nil {
		t.Fatal(err)
	}
	data, err := EncryptConfigExport(Config{}, key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var file ConfigExportFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	file.Version = 99
	data, err = json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptConfigExport(data, key); err == nil || !strings.Contains(err.Error(), "unsupported config export version") {
		t.Fatalf("err = %v", err)
	}
}

func TestMergeImportedConfigRejectsConflicts(t *testing.T) {
	current := Config{
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root"}},
		Hosts:        []Host{{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa"}},
	}
	imported := Config{
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "ops"}},
		Hosts:        []Host{{Name: "newhost", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa"}},
	}
	if _, _, err := MergeImportedConfig(current, imported, ImportMerge); err == nil {
		t.Fatal("expected conflict")
	}
}

func TestMergeImportedConfigKeepsExistingValues(t *testing.T) {
	current := Config{
		LogsPath:     "current-logs",
		Defaults:     Defaults{User: "root"},
		Groups:       map[string]GroupDefaults{"current": {User: "root"}},
		AuthProfiles: []AuthProfile{{Name: "current", User: "root"}},
		Hosts:        []Host{{Name: "current", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa"}},
	}
	imported := Config{
		LogsPath:     "imported-logs",
		Defaults:     Defaults{User: "ops", Port: 2222},
		Groups:       map[string]GroupDefaults{"imported": {User: "ops", Port: 2200}},
		AuthProfiles: []AuthProfile{{Name: "imported", User: "ops"}},
		Hosts:        []Host{{Name: "imported", IP: "10.0.0.9", User: "ops", KeyPath: "~/.ssh/id_ed25519"}},
	}
	merged, result, err := MergeImportedConfig(current, imported, ImportMerge)
	if err != nil {
		t.Fatal(err)
	}
	if merged.LogsPath != "current-logs" || merged.Defaults.User != "root" || merged.Defaults.Port != 2222 {
		t.Fatalf("merged defaults = %+v logs=%q", merged.Defaults, merged.LogsPath)
	}
	if result.AuthAdded != 1 || result.HostsAdded != 1 || len(merged.AuthProfiles) != 2 || len(merged.Hosts) != 2 {
		t.Fatalf("result=%+v merged=%+v", result, merged)
	}
	if result.GroupsAdded != 1 || len(merged.Groups) != 2 || merged.Groups["imported"].Port != 2200 {
		t.Fatalf("groups result=%+v groups=%+v", result, merged.Groups)
	}
}

func TestOverwriteImportedConfigUpdatesConflicts(t *testing.T) {
	current := Config{
		LogsPath:     "current-logs",
		Defaults:     Defaults{User: "root"},
		Groups:       map[string]GroupDefaults{"testing": {User: "root"}},
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root"}},
		Hosts:        []Host{{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa", Remark: "old"}},
	}
	imported := Config{
		LogsPath:     "imported-logs",
		Defaults:     Defaults{User: "ops"},
		Groups:       map[string]GroupDefaults{"testing": {User: "ops", Port: 2222}},
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "ops"}},
		Hosts:        []Host{{Name: "devhost", IP: "10.0.0.8", User: "ops", KeyPath: "~/.ssh/id_ed25519", Remark: "new"}},
	}
	merged, result, err := MergeImportedConfig(current, imported, ImportOverwrite)
	if err != nil {
		t.Fatal(err)
	}
	if merged.LogsPath != "imported-logs" || merged.Defaults.User != "ops" {
		t.Fatalf("merged = %+v", merged)
	}
	if result.AuthUpdated != 1 || result.HostsUpdated != 1 || result.GroupsUpdated != 1 {
		t.Fatalf("result = %+v", result)
	}
	if merged.AuthProfiles[0].User != "ops" || merged.Hosts[0].Remark != "new" || merged.Groups["testing"].Port != 2222 {
		t.Fatalf("merged entries = %+v %+v", merged.AuthProfiles, merged.Hosts)
	}
}

func TestReplaceImportedConfigReplacesAll(t *testing.T) {
	current := Config{Hosts: []Host{{Name: "old", IP: "10.0.0.8"}}}
	imported := Config{
		LogsPath:     "imported",
		Groups:       map[string]GroupDefaults{"testing": {User: "root"}},
		AuthProfiles: []AuthProfile{{Name: "dev-root"}},
		Hosts:        []Host{{Name: "new", IP: "10.0.0.9"}},
	}
	merged, result, err := MergeImportedConfig(current, imported, ImportReplace)
	if err != nil {
		t.Fatal(err)
	}
	if merged.LogsPath != "imported" || len(merged.Hosts) != 1 || merged.Hosts[0].Name != "new" {
		t.Fatalf("merged = %+v", merged)
	}
	if result.HostsAdded != 1 || result.AuthAdded != 1 || result.GroupsAdded != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestBackupConfigFile(t *testing.T) {
	path := withTempConfig(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"version":1,"hosts":[]}` + "\n")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	backup, err := BackupConfigFile(time.Date(2026, 7, 5, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" || !strings.Contains(backup, filepath.Join("backups", "sshc.config.20260705-100000.json")) {
		t.Fatalf("backup = %q", backup)
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(content) {
		t.Fatalf("backup content = %s", data)
	}
}
