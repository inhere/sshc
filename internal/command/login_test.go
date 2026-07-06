package command

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/inhere/sshc/internal/core"
)

func TestLoginPassesJumpOption(t *testing.T) {
	saveJumpCommandHosts(t)

	var gotHost core.Host
	t.Cleanup(setLoginRemoteForTest(func(host core.Host, opts core.LoginOptions) error {
		gotHost = host
		return nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"login", "inner-db", "--jump", "bastion"}); err != nil {
		t.Fatalf("login with jump: %v", err)
	}
	if gotHost.Jump != "bastion" {
		t.Fatalf("jump = %q, want bastion", gotHost.Jump)
	}
}

func TestLoginSelectsHostWhenTargetEmpty(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{
		{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22},
		{Name: "prodhost", IP: "10.0.0.9", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22},
	}}); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(setSelectLoginHostForTest(func(hosts []core.Host, _ io.Reader, _ io.Writer) (core.Host, error) {
		if len(hosts) != 2 {
			t.Fatalf("hosts len = %d, want 2", len(hosts))
		}
		return hosts[1], nil
	}))

	var gotHost core.Host
	t.Cleanup(setLoginRemoteForTest(func(host core.Host, opts core.LoginOptions) error {
		gotHost = host
		return nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"login"}); err != nil {
		t.Fatalf("login: %v", err)
	}
	if gotHost.Name != "prodhost" || gotHost.IP != "10.0.0.9" {
		t.Fatalf("host = %+v", gotHost)
	}
}

func TestLoginSelectsHostWhenTargetMatchesMultiple(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "ops", User: "root", KeyPath: "~/.ssh/id_rsa"}},
		Hosts: []core.Host{
			{Name: "testing-web", IP: "10.0.0.8", AuthRef: "ops", Port: 22, Group: "testing"},
			{Name: "testing-db", IP: "10.0.0.9", AuthRef: "ops", Port: 22, Group: "testing"},
			{Name: "prod-db", IP: "10.0.0.10", AuthRef: "ops", Port: 22, Group: "prod"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(setSelectLoginHostForTest(func(hosts []core.Host, _ io.Reader, _ io.Writer) (core.Host, error) {
		if len(hosts) != 2 || hosts[0].Name != "testing-web" || hosts[1].Name != "testing-db" {
			t.Fatalf("hosts = %+v", hosts)
		}
		return hosts[1], nil
	}))

	var gotHost core.Host
	t.Cleanup(setLoginRemoteForTest(func(host core.Host, opts core.LoginOptions) error {
		gotHost = host
		return nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"login", "testing", "--jump", "prod-db"}); err != nil {
		t.Fatalf("login: %v", err)
	}
	if gotHost.Name != "testing-db" || gotHost.User != "root" || gotHost.Jump != "prod-db" {
		t.Fatalf("host = %+v", gotHost)
	}
}

func TestLoginCommandProxyPassesHostAndWritesLog(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{
		{Name: "pve-host", IP: "192.168.1.20", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22, HostKeyCheck: core.HostKeyCheckInsecure},
		{Name: "lxc-app", Backend: core.HostBackendCommandProxy, Via: "pve-host", LoginCommand: "pct enter 101", Group: "lxc"},
	}}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	var gotHost core.Host
	t.Cleanup(setLoginRemoteForTest(func(host core.Host, opts core.LoginOptions) error {
		gotHost = host
		return nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"login", "lxc-app"}); err != nil {
		t.Fatalf("login command_proxy: %v", err)
	}
	if gotHost.Backend != core.HostBackendCommandProxy || gotHost.Via != "pve-host" || gotHost.LoginCommand != "pct enter 101" {
		t.Fatalf("host = %+v", gotHost)
	}
	if !strings.Contains(out.String(), "connecting to lxc-app (command_proxy via:pve-host)") {
		t.Fatalf("output = %q", out.String())
	}
	lines, err := core.ReadRunLogs("lxc-app", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 ||
		!strings.Contains(lines[0], `"backend":"command_proxy"`) ||
		!strings.Contains(lines[0], `"via":"pve-host"`) ||
		!strings.Contains(lines[0], `"proxied_command":"pct enter 101"`) {
		t.Fatalf("logs = %#v", lines)
	}
}

func TestLoginCommandProxyRejectsJumpOption(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{
		{Name: "pve-host", IP: "192.168.1.20", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22},
		{Name: "lxc-app", Backend: core.HostBackendCommandProxy, Via: "pve-host", LoginCommand: "pct enter 101"},
	}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	err := app.RunWithArgs([]string{"login", "lxc-app", "--jump", "bastion"})
	if err == nil || !strings.Contains(err.Error(), "--jump is not supported for command_proxy") {
		t.Fatalf("err = %v", err)
	}
}
