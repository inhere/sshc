package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
