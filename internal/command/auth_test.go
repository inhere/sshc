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

func TestAuthAddEmbedsKeyAndPassphraseFromClipboard(t *testing.T) {
	path := withTempConfig(t)
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	keyContent := "FAKE_AUTH_PRIVATE_KEY_CONTENT_67890\n"
	if err := os.WriteFile(keyPath, []byte(keyContent), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(setReadClipboardForTest(func() (string, error) {
		return " clip-secret \n", nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"auth", "add", "deploy-key", "-u", "deploy", "--key", keyPath, "--embed-key", "--key-passphrase", "clip"}); err != nil {
		t.Fatalf("auth add embedded key: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if strings.Contains(content, keyContent) || strings.Contains(content, "clip-secret") {
		t.Fatalf("stored config leaked embedded key or passphrase: %s", content)
	}
	if !strings.Contains(content, `"key_data_enc": "v1:`) || !strings.Contains(content, `"key_passphrase_enc": "v1:`) {
		t.Fatalf("stored config missing encrypted key fields: %s", content)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AuthProfiles) != 1 || config.AuthProfiles[0].KeyData != keyContent || config.AuthProfiles[0].KeyPassphrase != "clip-secret" {
		t.Fatalf("auth profiles = %+v", config.AuthProfiles)
	}
}

func TestAuthAddKeyPassphraseSpaceSourceAfterName(t *testing.T) {
	withTempConfig(t)
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("FAKE_AUTH_PRIVATE_KEY_CONTENT_ENV\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(keyPassphraseEnvKey, "env-secret")

	app := newTestApp()
	if err := app.RunWithArgs([]string{"auth", "add", "deploy-key", "--key", keyPath, "--key-passphrase", "env"}); err != nil {
		t.Fatalf("auth add key passphrase: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AuthProfiles) != 1 || config.AuthProfiles[0].Name != "deploy-key" || config.AuthProfiles[0].KeyPassphrase != "env-secret" {
		t.Fatalf("auth profiles = %+v", config.AuthProfiles)
	}
}

func TestAuthAddSourceLikeProfileNameStillWorks(t *testing.T) {
	withTempConfig(t)
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("FAKE_AUTH_PRIVATE_KEY_CONTENT_CLIP\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(setReadClipboardForTest(func() (string, error) {
		return "clip-secret", nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"auth", "add", "env", "--key", keyPath, "--key-passphrase", "clip"}); err != nil {
		t.Fatalf("auth add key passphrase: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AuthProfiles) != 1 || config.AuthProfiles[0].Name != "env" || config.AuthProfiles[0].KeyPassphrase != "clip-secret" {
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
		Name:          "dev-root",
		User:          "root",
		Password:      "secret",
		KeyData:       "key-data",
		KeyPassphrase: "key-secret",
		Remark:        "shared root login",
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
	if strings.Contains(out.String(), "secret") || strings.Contains(out.String(), "key-data") || !strings.Contains(out.String(), `"password_enc": "***"`) || !strings.Contains(out.String(), `"key_data_enc": "***"`) || !strings.Contains(out.String(), `"key_passphrase_enc": "***"`) || !strings.Contains(out.String(), `"remark": "shared root login"`) {
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

func TestAuthProfileTypeUsesEmbeddedKey(t *testing.T) {
	if got := authProfileType(core.AuthProfile{KeyDataEnc: "v1:key"}); got != "key" {
		t.Fatalf("authProfileType embedded key = %q, want key", got)
	}
	if err := validateAuthProfile(core.AuthProfile{Name: "deploy-key", KeyDataEnc: "v1:key"}); err != nil {
		t.Fatalf("validate embedded key profile: %v", err)
	}
}
