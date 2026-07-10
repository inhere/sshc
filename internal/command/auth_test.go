package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inhere/sshc/internal/core"
)

func TestAuthAddPasswordProfile(t *testing.T) {
	path := withTempConfig(t)
	t.Cleanup(setReadInteractivePasswordForTest(func(...string) string { return " secret " }))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"auth", "add", "dev-root", "-u", "root", "-p", "--remark", "shared root login"}); err != nil {
		t.Fatalf("auth add: %v", err)
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

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AuthProfiles) != 1 || config.AuthProfiles[0].Password != "secret" {
		t.Fatalf("auth profiles = %+v", config.AuthProfiles)
	}
	if config.AuthProfiles[0].Remark != "shared root login" {
		t.Fatalf("auth remark = %q", config.AuthProfiles[0].Remark)
	}
}

func TestAuthAddRejectsInlinePasswordValue(t *testing.T) {
	withTempConfig(t)
	t.Cleanup(setReadInteractivePasswordForTest(func(...string) string { return "secret" }))

	app := newTestApp()
	err := app.RunWithArgs([]string{"auth", "add", "dev-root", "-u", "root", "-p", "secret"})
	if err == nil || !strings.Contains(err.Error(), "does not accept an inline value") {
		t.Fatalf("err = %v", err)
	}
}

func TestAuthAddKeyProfile(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	if err := app.RunWithArgs([]string{"auth", "add", "deploy-key", "-u", "deploy", "--key", "~/.ssh/id_ed25519"}); err != nil {
		t.Fatalf("auth add key: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AuthProfiles) != 1 || config.AuthProfiles[0].KeyPath != "~/.ssh/id_ed25519" {
		t.Fatalf("auth profiles = %+v", config.AuthProfiles)
	}
}

func TestAuthAddStoresRelativeKeyPathAsAbsolute(t *testing.T) {
	withTempConfig(t)

	cwd := t.TempDir()
	t.Chdir(cwd)
	relKey := filepath.Join("keys", "id_ed25519")
	want, err := filepath.Abs(relKey)
	if err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"auth", "add", "deploy-key", "-u", "deploy", "--key", relKey}); err != nil {
		t.Fatalf("auth add key: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AuthProfiles) != 1 || config.AuthProfiles[0].KeyPath != want {
		t.Fatalf("auth profiles = %+v, want key path %q", config.AuthProfiles, want)
	}
}

func TestAuthListAndShowMaskSecrets(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{AuthProfiles: []core.AuthProfile{{
		Name:     "dev-root",
		User:     "root",
		Password: "secret",
		Remark:   "shared root login",
	}}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	if err := app.RunWithArgs([]string{"auth", "list"}); err != nil {
		t.Fatalf("auth list: %v", err)
	}
	if !strings.Contains(out.String(), "dev-root") || !strings.Contains(out.String(), "shared root login") || strings.Contains(out.String(), "secret") {
		t.Fatalf("list output = %q", out.String())
	}

	out.Reset()
	if err := app.RunWithArgs([]string{"auth", "show", "dev-root"}); err != nil {
		t.Fatalf("auth show: %v", err)
	}
	if strings.Contains(out.String(), "secret") || !strings.Contains(out.String(), `"password_enc": "***"`) || !strings.Contains(out.String(), `"remark": "shared root login"`) {
		t.Fatalf("show output = %q", out.String())
	}
}

func TestAuthRemoveRefusedWhenUsedByHost(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}},
		Hosts:        []core.Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root"}},
	}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	err := app.RunWithArgs([]string{"auth", "rm", "dev-root", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "is used by host") {
		t.Fatalf("err = %v", err)
	}
}

func TestAuthRemoveWithYes(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"auth", "rm", "dev-root", "--yes"}); err != nil {
		t.Fatalf("auth rm: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AuthProfiles) != 0 {
		t.Fatalf("auth profiles = %+v", config.AuthProfiles)
	}
}
