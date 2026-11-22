package core

import (
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
)

// fakeSecretCmd replaces the shell runner and counts invocations.
func fakeSecretCmd(t *testing.T, secret string, err error) *int {
	t.Helper()

	calls := 0
	old := secretCmdRun
	t.Cleanup(func() {
		secretCmdRun = old
		resetSecretCmdCacheForTest()
	})
	resetSecretCmdCacheForTest()
	secretCmdRun = func(_ string, _ time.Duration) (string, error) {
		calls++
		return secret, err
	}
	return &calls
}

func TestResolveSecretCommand(t *testing.T) {
	t.Run("result is cached per command", func(t *testing.T) {
		is := assert.New(t)
		calls := fakeSecretCmd(t, "s3cret", nil)

		first, err := ResolveSecretCommand("gopass show -on ssh/dev", 0)
		is.NoErr(err)
		is.Eq("s3cret", first)

		second, err := ResolveSecretCommand("gopass show -on ssh/dev", 0)
		is.NoErr(err)
		is.Eq("s3cret", second)
		is.Eq(1, *calls)

		_, err = ResolveSecretCommand("gopass show -on ssh/other", 0)
		is.NoErr(err)
		is.Eq(2, *calls)
	})

	t.Run("failures are not cached", func(t *testing.T) {
		is := assert.New(t)
		calls := fakeSecretCmd(t, "", errors.New("secret command failed: exit status 1"))

		_, err := ResolveSecretCommand("gopass show missing", 0)
		is.Err(err)
		_, err = ResolveSecretCommand("gopass show missing", 0)
		is.Err(err)
		is.Eq(2, *calls)
	})

	t.Run("empty command is rejected", func(t *testing.T) {
		is := assert.New(t)
		_, err := ResolveSecretCommand("   ", 0)
		is.Err(err)
		is.Contains(err.Error(), "empty")
	})
}

func TestRunSecretShellCommand(t *testing.T) {
	if _, args := secretShellCommand("echo probe"); len(args) == 0 {
		t.Skipf("no supported shell on %s", runtime.GOOS)
	}

	t.Run("single line output is trimmed", func(t *testing.T) {
		is := assert.New(t)

		secret, err := runSecretShellCommand("echo demo-secret", 5*time.Second)
		is.NoErr(err)
		is.Eq("demo-secret", secret)
		is.True(!strings.Contains(secret, "\n"))
	})

	t.Run("empty output fails", func(t *testing.T) {
		is := assert.New(t)

		_, err := normalizeSecretCommandOutput(nil)
		is.Err(err)
		is.Contains(err.Error(), "no output")

		_, err = normalizeSecretCommandOutput([]byte("\r\n"))
		is.Err(err)
		is.Contains(err.Error(), "no output")
	})

	t.Run("multiline output fails", func(t *testing.T) {
		is := assert.New(t)

		_, err := normalizeSecretCommandOutput([]byte("a\nb\n"))
		is.Err(err)
		is.Contains(err.Error(), "multiple lines")
	})

	t.Run("trailing newlines are trimmed", func(t *testing.T) {
		is := assert.New(t)

		secret, err := normalizeSecretCommandOutput([]byte("pass word\r\n"))
		is.NoErr(err)
		is.Eq("pass word", secret)
	})
}

func TestResolveHostSecrets(t *testing.T) {
	t.Run("password command fills the runtime field", func(t *testing.T) {
		is := assert.New(t)
		calls := fakeSecretCmd(t, "from-gopass", nil)

		host, err := ResolveHostSecrets(Host{
			Name:        "dev",
			IP:          "10.0.0.8",
			User:        "root",
			PasswordCmd: "gopass show -on ssh/dev",
		})
		is.NoErr(err)
		is.Eq("from-gopass", host.Password)
		is.Eq("", host.PasswordCmd)
		is.Eq(1, *calls)

		// A second resolution runs nothing.
		_, err = ResolveHostSecrets(host)
		is.NoErr(err)
		is.Eq(1, *calls)
	})

	t.Run("passphrase command fills the runtime field", func(t *testing.T) {
		is := assert.New(t)
		fakeSecretCmd(t, "phrase", nil)

		host, err := ResolveHostSecrets(Host{
			Name:             "dev",
			IP:               "10.0.0.8",
			User:             "root",
			KeyPath:          "~/.ssh/id_ed25519",
			KeyPassphraseCmd: "sops -d phrase.yaml",
		})
		is.NoErr(err)
		is.Eq("phrase", host.KeyPassphrase)
		is.Eq("", host.KeyPassphraseCmd)
	})

	t.Run("command failure keeps the host unchanged", func(t *testing.T) {
		is := assert.New(t)
		fakeSecretCmd(t, "", errors.New("secret command failed: exit status 2"))

		host, err := ResolveHostSecrets(Host{Name: "dev", IP: "10.0.0.8", User: "root", PasswordCmd: "false"})
		is.Err(err)
		is.Contains(err.Error(), "password_cmd")
		is.Eq("", host.Password)
	})

	t.Run("no commands is a no-op", func(t *testing.T) {
		is := assert.New(t)
		calls := fakeSecretCmd(t, "unused", nil)

		host, err := ResolveHostSecrets(Host{Name: "dev", IP: "10.0.0.8", User: "root", Password: "stored"})
		is.NoErr(err)
		is.Eq("stored", host.Password)
		is.Eq(0, *calls)
	})
}
