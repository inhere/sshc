package command

import (
	"strings"
	"testing"

	"github.com/inhere/sshc/internal/core"
)

func TestHostSetUpdatesFields(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "ops", User: "root", KeyPath: "~/.ssh/id_rsa"}},
		Hosts: []core.Host{
			{Name: "devhost", IP: "10.0.0.8", AuthRef: "ops", Port: 22},
			{Name: "bastion", IP: "10.0.0.9", AuthRef: "ops", Port: 22},
		},
	}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{
		"host", "set", "devhost",
		"--ip", "10.0.0.10",
		"--user", "deploy",
		"--key", "~/.ssh/deploy",
		"--remark", "testing host",
		"--group", "testing",
		"--jump", "bastion",
		"--port", "2222",
		"--connect-timeout", "10s",
		"--run-timeout", "1m",
		"--remote-script-dir", "/var/tmp",
		"--host-key-check", core.HostKeyCheckInsecure,
		"--known-hosts-path", "~/.ssh/known_hosts",
	}); err != nil {
		t.Fatalf("host set: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	host := config.Hosts[0]
	if host.IP != "10.0.0.10" || host.User != "deploy" || host.KeyPath != "~/.ssh/deploy" || host.Port != 2222 {
		t.Fatalf("host = %+v", host)
	}
	if host.Remark != "testing host" || host.Group != "testing" || host.Jump != "bastion" || host.ConnectTimeout != "10s" || host.RunTimeout != "1m" {
		t.Fatalf("host metadata = %+v", host)
	}
	if host.RemoteScriptDir != "/var/tmp" || host.HostKeyCheck != core.HostKeyCheckInsecure || host.KnownHostsPath != "~/.ssh/known_hosts" {
		t.Fatalf("host defaults = %+v", host)
	}
}

func TestHostUnsetFields(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		Defaults: core.Defaults{User: "root"},
		AuthProfiles: []core.AuthProfile{
			{Name: "ops", KeyPath: "~/.ssh/id_rsa"},
		},
		Hosts: []core.Host{{
			Name:            "devhost",
			IP:              "10.0.0.8",
			User:            "deploy",
			AuthRef:         "ops",
			KeyPath:         "~/.ssh/deploy",
			Remark:          "testing host",
			Group:           "testing",
			Port:            22,
			ConnectTimeout:  "10s",
			RunTimeout:      "1m",
			RemoteScriptDir: "/var/tmp",
			HostKeyCheck:    core.HostKeyCheckInsecure,
			KnownHostsPath:  "~/.ssh/known_hosts",
		}},
	}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{
		"host", "unset", "devhost",
		"--user",
		"--key",
		"--remark",
		"--group",
		"--connect-timeout",
		"--run-timeout",
		"--remote-script-dir",
		"--host-key-check",
		"--known-hosts-path",
	}); err != nil {
		t.Fatalf("host unset: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	host := config.Hosts[0]
	if host.User != "" || host.KeyPath != "" || host.Remark != "" || host.Group != "" {
		t.Fatalf("host = %+v", host)
	}
	if host.ConnectTimeout != "" || host.RunTimeout != "" || host.RemoteScriptDir != "" || host.HostKeyCheck != "" || host.KnownHostsPath != "" {
		t.Fatalf("host defaults = %+v", host)
	}
	if host.AuthRef != "ops" {
		t.Fatalf("auth ref = %q, want ops", host.AuthRef)
	}
}

func TestHostUnsetRejectsInvalidAuth(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{
		Name:    "devhost",
		IP:      "10.0.0.8",
		User:    "root",
		KeyPath: "~/.ssh/id_rsa",
		Port:    22,
	}}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	err := app.RunWithArgs([]string{"host", "unset", "devhost", "--user", "--key"})
	if err == nil || !strings.Contains(err.Error(), "user is required") {
		t.Fatalf("err = %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Hosts[0].User != "root" || config.Hosts[0].KeyPath != "~/.ssh/id_rsa" {
		t.Fatalf("host was changed: %+v", config.Hosts[0])
	}
}

func TestHostSetRejectsDuplicateIP(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{
		{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22},
		{Name: "prodhost", IP: "10.0.0.9", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22},
	}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	err := app.RunWithArgs([]string{"host", "set", "devhost", "--ip", "10.0.0.9"})
	if err == nil || !strings.Contains(err.Error(), "host ip") {
		t.Fatalf("err = %v", err)
	}
}
