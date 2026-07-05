package command

import (
	"io"
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
