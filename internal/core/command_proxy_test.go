package core

import (
	"context"
	"strings"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestRenderCommandProxyRunQuotesCommand(t *testing.T) {
	got, err := RenderCommandProxyRun("pct exec 101 -- sh -lc {{cmd}}", "cd /opt/app && echo a'b")
	if err != nil {
		t.Fatal(err)
	}
	want := `pct exec 101 -- sh -lc 'cd /opt/app && echo a'\''b'`
	if got != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
}

func TestRenderCommandProxyRunRejectsMissingToken(t *testing.T) {
	_, err := RenderCommandProxyRun("pct exec 101 -- hostname", "hostname")
	if err == nil || !strings.Contains(err.Error(), "must contain {{cmd}}") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanCommandProxyRunUsesViaHost(t *testing.T) {
	withTempConfig(t)
	if err := SaveConfig(&Config{Hosts: []Host{
		{Name: "pve-host", IP: "192.168.1.20", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22, HostKeyCheck: HostKeyCheckInsecure},
		{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host", RunTemplate: "pct exec 101 -- sh -lc {{cmd}}"},
	}}); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanCommandProxyRun(Host{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host", RunTemplate: "pct exec 101 -- sh -lc {{cmd}}"}, "hostname", RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Via.Name != "pve-host" || plan.ProxiedCommand != "pct exec 101 -- sh -lc 'hostname'" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestExecuteCommandProxyRunsOnViaHost(t *testing.T) {
	withTempConfig(t)
	if err := SaveConfig(&Config{Hosts: []Host{
		{Name: "pve-host", IP: "192.168.1.20", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22, HostKeyCheck: HostKeyCheckInsecure},
		{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host", RunTemplate: "pct exec 101 -- sh -lc {{cmd}}"},
	}}); err != nil {
		t.Fatal(err)
	}

	var gotHost Host
	var gotCommand string
	old := newCommandProxySSHClient
	newCommandProxySSHClient = func(host Host) (RemoteClient, error) {
		gotHost = host
		return &fakeRemoteClient{run: func(command string) ([]byte, error) {
			gotCommand = command
			return []byte("ok\n"), nil
		}}, nil
	}
	defer func() { newCommandProxySSHClient = old }()

	out, err := ExecuteCommandProxy(Host{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host", RunTemplate: "pct exec 101 -- sh -lc {{cmd}}"}, "hostname", RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "ok\n" || gotHost.Name != "pve-host" || gotCommand != "pct exec 101 -- sh -lc 'hostname'" {
		t.Fatalf("out=%q host=%+v command=%q", string(out), gotHost, gotCommand)
	}
}

func TestExecuteCommandProxyRejectsScript(t *testing.T) {
	_, err := ExecuteCommandProxy(Host{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host", RunTemplate: "pct exec 101 -- sh -lc {{cmd}}"}, "", RunOptions{ScriptPath: "deploy.sh"})
	if err == nil || !strings.Contains(err.Error(), "--script is not supported") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanCommandProxyLoginUsesViaHost(t *testing.T) {
	withTempConfig(t)
	if err := SaveConfig(&Config{Hosts: []Host{
		{Name: "pve-host", IP: "192.168.1.20", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22, HostKeyCheck: HostKeyCheckInsecure},
		{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host", LoginCommand: "pct enter 101"},
	}}); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanCommandProxyLogin(Host{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host", LoginCommand: "pct enter 101"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Via.Name != "pve-host" || plan.LoginCommand != "pct enter 101" {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestPlanCommandProxyLoginRejectsMissingLoginCommand(t *testing.T) {
	_, err := PlanCommandProxyLogin(Host{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host"})
	if err == nil || !strings.Contains(err.Error(), "login_command is required") {
		t.Fatalf("err = %v", err)
	}
}

type fakeRemoteClient struct {
	run func(string) ([]byte, error)
}

func (f *fakeRemoteClient) Run(command string) ([]byte, error) {
	if f.run != nil {
		return f.run(command)
	}
	return nil, nil
}

func (f *fakeRemoteClient) RunContext(_ context.Context, command string) ([]byte, error) {
	return f.Run(command)
}

func (f *fakeRemoteClient) NewSession() (*ssh.Session, error) {
	return nil, nil
}

func (f *fakeRemoteClient) NewSftp(...sftp.ClientOption) (*sftp.Client, error) {
	return nil, nil
}

func (f *fakeRemoteClient) Close() error {
	return nil
}
