package command

import (
	"bytes"
	"fmt"
	"net"
	"testing"

	"github.com/gookit/gcli/v3"
	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/sshc/internal/core"
)

func TestAddHostPasswordCommand(t *testing.T) {
	t.Run("stores the command instead of a secret", func(t *testing.T) {
		is := assert.New(t)
		withTempConfig(t)

		err := newTestApp().RunWithArgs([]string{"add", "--ip", "10.0.0.8", "-u", "root", "--name", "dev",
			"--password-cmd", "gopass show -on ssh/dev"})
		is.NoErr(err)

		store := readTestStore(t)
		is.Len(store.Hosts, 1)
		is.Eq("gopass show -on ssh/dev", store.Hosts[0].PasswordCmd)
		is.Eq("", store.Hosts[0].Password)
		is.Eq("cmd", core.AuthLabel(store.Hosts[0]))
	})

	t.Run("key passphrase command requires a key", func(t *testing.T) {
		is := assert.New(t)
		withTempConfig(t)

		err := newTestApp().RunWithArgs([]string{"add", "--ip", "10.0.0.8", "-u", "root", "--name", "dev",
			"--key-passphrase-cmd", "sops -d pass"})
		is.Err(err)
		is.Contains(err.Error(), "requires --key")
	})

	t.Run("password and command conflict", func(t *testing.T) {
		is := assert.New(t)
		withTempConfig(t)

		err := newTestApp().RunWithArgs([]string{"add", "--ip", "10.0.0.8", "-u", "root", "--name", "dev",
			"-p", "secret", "--password-cmd", "gopass show ssh/dev"})
		is.Err(err)
		is.Contains(err.Error(), "cannot be used together")

		config, loadErr := core.LoadConfig()
		is.NoErr(loadErr)
		is.Len(config.Hosts, 0)
	})
}

func TestAuthAddPasswordCommand(t *testing.T) {
	t.Run("stores the command on the profile", func(t *testing.T) {
		is := assert.New(t)
		withTempConfig(t)

		err := newTestApp().RunWithArgs([]string{"auth", "add", "ops", "-u", "root", "--password-cmd", "gopass show -on ssh/ops"})
		is.NoErr(err)

		config, loadErr := core.LoadConfig()
		is.NoErr(loadErr)
		is.Len(config.AuthProfiles, 1)
		is.Eq("gopass show -on ssh/ops", config.AuthProfiles[0].PasswordCmd)
		is.Eq("", config.AuthProfiles[0].Password)
	})

	t.Run("password prompt and command conflict", func(t *testing.T) {
		is := assert.New(t)
		withTempConfig(t)

		err := newTestApp().RunWithArgs([]string{"auth", "add", "ops", "-u", "root", "-p", "--password-cmd", "gopass show ssh/ops"})
		is.Err(err)
		is.Contains(err.Error(), "cannot be used together")
	})
}

func TestSecretCommandFlagConflict(t *testing.T) {
	is := assert.New(t)
	withTempConfig(t)

	err := newTestApp().RunWithArgs([]string{"add", "--ip", "10.0.0.8", "-u", "root", "--name", "dev",
		"--key", "~/.ssh/id_rsa", "--key-passphrase", "env", "--key-passphrase-cmd", "sops -d pass"})
	is.Err(err)
	is.Contains(err.Error(), "cannot be used together")
}

func TestCheckNoSecretCmd(t *testing.T) {
	is := assert.New(t)
	withTempConfig(t)

	// A real listener keeps the TCP step green so the auth step is reached.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	is.NoErr(err)
	defer listener.Close()
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	is.NoErr(err)
	port := 0
	_, err = fmt.Sscan(portText, &port)
	is.NoErr(err)

	is.NoErr(core.SaveConfig(&core.Config{Hosts: []core.Host{{
		Name:         "dev",
		IP:           "127.0.0.1",
		User:         "root",
		Port:         port,
		PasswordCmd:  "gopass show ssh/dev",
		HostKeyCheck: core.HostKeyCheckInsecure,
	}}}))

	buf := new(bytes.Buffer)
	restore := setCommandOutputForTest(buf)
	code := newTestApp().Run([]string{"check", "dev", "--no-secret-cmd", "--json"})
	restore()

	// A skipped secret command is not a check failure.
	is.Eq(0, code)
	out := buf.String()
	is.Contains(out, `"auth": "-"`)
	is.Contains(out, "secret command skipped")
}

var _ = gcli.ERR
