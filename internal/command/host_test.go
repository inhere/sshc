package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inhere/sshc/internal/core"
)

func TestHostImportIPsCommand(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}},
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "hosts.ips")
	if err := os.WriteFile(path, []byte("10.0.0.8\n10.0.0.9\n"), 0600); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "import", "-f", path, "--auth", "dev-root", "--group", "testing", "--tags", "imported,testing", "--yes"}); err != nil {
		t.Fatalf("host import ips: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Hosts) != 2 || config.Hosts[0].IP != "10.0.0.8" || config.Hosts[1].Group != "testing" {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
	if config.Hosts[0].Port != core.DefaultSSHPort || config.Hosts[1].Port != core.DefaultSSHPort {
		t.Fatalf("ports = %d,%d", config.Hosts[0].Port, config.Hosts[1].Port)
	}
	if strings.Join(config.Hosts[0].Tags, ",") != "imported,testing" {
		t.Fatalf("tags = %+v", config.Hosts[0].Tags)
	}
}

func TestHostImportPlainCommand(t *testing.T) {
	withTempConfig(t)
	input := `ip=10.0.0.8
name=devhost
user=root
key=~/.ssh/id_rsa
group=testing

ip=10.0.0.9
name=dbhost
user=root
key=~/.ssh/id_rsa
group=testing
`
	path := filepath.Join(t.TempDir(), "hosts.txt")
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "import", "-f", path, "--format", "plain", "--yes"}); err != nil {
		t.Fatalf("host import plain: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Hosts) != 2 || config.Hosts[0].Name != "devhost" || config.Hosts[1].Name != "dbhost" {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestHostImportPlainCommandProxy(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{Name: "pve-host", IP: "192.168.1.20", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22}}}); err != nil {
		t.Fatal(err)
	}
	input := `name=lxc-app
backend=command_proxy
via=pve-host
run_template=pct exec 101 -- sh -lc {{cmd}}
login_command=pct enter 101
group=lxc
`
	path := filepath.Join(t.TempDir(), "hosts.txt")
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "import", "-f", path, "--format", "plain", "--yes"}); err != nil {
		t.Fatalf("host import plain command_proxy: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	host := config.Hosts[1]
	if host.Name != "lxc-app" || host.Backend != core.HostBackendCommandProxy || host.Via != "pve-host" || host.RunTemplate == "" || host.LoginCommand != "pct enter 101" {
		t.Fatalf("host = %+v", host)
	}
}

func TestHostImportCSVCommand(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}},
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "hosts.csv")
	if err := os.WriteFile(path, []byte("name,ip,auth,group,remark,port\ndevhost,10.0.0.8,dev-root,testing,app server,2222\n"), 0600); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "import", "-f", path, "--yes"}); err != nil {
		t.Fatalf("host import csv: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	host := config.Hosts[0]
	if host.Name != "devhost" || host.AuthRef != "dev-root" || host.Port != 2222 || host.Remark != "app server" {
		t.Fatalf("host = %+v", host)
	}
}

func TestHostImportDryRunDoesNotSave(t *testing.T) {
	withTempConfig(t)
	path := filepath.Join(t.TempDir(), "hosts.txt")
	if err := os.WriteFile(path, []byte("ip=10.0.0.8\nuser=root\nkey=~/.ssh/id_rsa\n"), 0600); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "import", "-f", path, "--dry-run"}); err != nil {
		t.Fatalf("host import dry-run: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Hosts) != 0 {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestHostImportRejectsConflicts(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa"}}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "hosts.txt")
	if err := os.WriteFile(path, []byte("ip=10.0.0.9\nname=devhost\nuser=root\nkey=~/.ssh/id_rsa\n"), 0600); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	err := app.RunWithArgs([]string{"host", "import", "-f", path, "--format", "plain", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("err = %v", err)
	}
}

func TestHostImportSkipExisting(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa"}}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "hosts.txt")
	if err := os.WriteFile(path, []byte("ip=10.0.0.8\nname=devhost\nuser=root\nkey=~/.ssh/id_rsa\n\nip=10.0.0.9\nname=newhost\nuser=root\nkey=~/.ssh/id_rsa\n"), 0600); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "import", "-f", path, "--format", "plain", "--skip-existing", "--yes"}); err != nil {
		t.Fatalf("host import skip: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Hosts) != 2 || config.Hosts[1].Name != "newhost" {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestHostImportOverwrite(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa", Remark: "old"}}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "hosts.txt")
	if err := os.WriteFile(path, []byte("ip=10.0.0.8\nname=devhost\nuser=root\nkey=~/.ssh/id_rsa\nremark=new\n"), 0600); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "import", "-f", path, "--format", "plain", "--overwrite", "--yes"}); err != nil {
		t.Fatalf("host import overwrite: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Hosts) != 1 || config.Hosts[0].Remark != "new" {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestHostImportFromClipboard(t *testing.T) {
	withTempConfig(t)
	t.Cleanup(setReadClipboardForTest(func() (string, error) {
		return "ip=10.0.0.8\nname=devhost\nuser=root\nkey=~/.ssh/id_rsa\n", nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "import", "--from-clipboard", "--format", "plain"}); err != nil {
		t.Fatalf("host import clipboard: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Hosts) != 1 || config.Hosts[0].Name != "devhost" {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestHostImportRequiresYesInNonInteractiveMode(t *testing.T) {
	withTempConfig(t)
	path := filepath.Join(t.TempDir(), "hosts.ips")
	if err := os.WriteFile(path, []byte("10.0.0.8\n10.0.0.9\n"), 0600); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	err := app.RunWithArgs([]string{"host", "import", "-f", path, "--user", "root", "--key", "~/.ssh/id_rsa"})
	if err == nil || !strings.Contains(err.Error(), "confirmation required") {
		t.Fatalf("err = %v", err)
	}
}

func TestHostImportEncryptsPassword(t *testing.T) {
	path := withTempConfig(t)
	csvPath := filepath.Join(t.TempDir(), "hosts.csv")
	if err := os.WriteFile(csvPath, []byte("name,ip,user,password\ndevhost,10.0.0.8,root,secret\n"), 0600); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "import", "-f", csvPath, "--yes"}); err != nil {
		t.Fatalf("host import password: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, `"password": "secret"`) || strings.Contains(content, `"password":"secret"`) {
		t.Fatalf("config contains plaintext password: %s", content)
	}
	if !strings.Contains(content, `"password_enc": "v1:`) {
		t.Fatalf("config missing encrypted password: %s", content)
	}
}

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
		"ip=10.0.0.10",
		"user=deploy",
		"key=~/.ssh/deploy",
		"remark=testing host",
		"group=testing",
		"tags=app,testing",
		"jump=bastion",
		"port=2222",
		"connect_timeout=10s",
		"run_timeout=1m",
		"remote_script_dir=/var/tmp",
		"host_key_check=" + core.HostKeyCheckInsecure,
		"known_hosts_path=~/.ssh/known_hosts",
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
	if strings.Join(host.Tags, ",") != "app,testing" {
		t.Fatalf("tags = %+v", host.Tags)
	}
	if host.RemoteScriptDir != "/var/tmp" || host.HostKeyCheck != core.HostKeyCheckInsecure || host.KnownHostsPath != "~/.ssh/known_hosts" {
		t.Fatalf("host defaults = %+v", host)
	}
}

func TestHostSetCommandProxyFields(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{
		{Name: "pve-host", IP: "192.168.1.20", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22},
		{Name: "lxc-app", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22},
	}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{
		"host", "set", "lxc-app",
		"backend=" + core.HostBackendCommandProxy,
		"via=pve-host",
		"run_template=pct exec 101 -- sh -lc {{cmd}}",
		"login_command=pct enter 101",
	}); err != nil {
		t.Fatalf("host set command_proxy: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	host := config.Hosts[1]
	if host.Backend != core.HostBackendCommandProxy || host.Via != "pve-host" || host.RunTemplate == "" || host.LoginCommand != "pct enter 101" {
		t.Fatalf("host = %+v", host)
	}
}

func TestHostUnsetCommandProxyFields(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{
		Name:         "lxc-app",
		IP:           "10.0.0.8",
		User:         "root",
		KeyPath:      "~/.ssh/id_rsa",
		Port:         22,
		Backend:      core.HostBackendCommandProxy,
		Via:          "pve-host",
		RunTemplate:  "pct exec 101 -- sh -lc {{cmd}}",
		LoginCommand: "pct enter 101",
	}}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "unset", "lxc-app", "backend", "via", "run_template", "login_command"}); err != nil {
		t.Fatalf("host unset command_proxy: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	host := config.Hosts[0]
	if host.Backend != "" || host.Via != "" || host.RunTemplate != "" || host.LoginCommand != "" {
		t.Fatalf("host = %+v", host)
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
			Tags:            []string{"app", "testing"},
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
		"user",
		"key",
		"remark",
		"group",
		"tags",
		"connect_timeout",
		"run_timeout",
		"remote_script_dir",
		"host_key_check",
		"known_hosts_path",
	}); err != nil {
		t.Fatalf("host unset: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	host := config.Hosts[0]
	if host.User != "" || host.KeyPath != "" || host.Remark != "" || host.Group != "" || len(host.Tags) != 0 {
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
	err := app.RunWithArgs([]string{"host", "unset", "devhost", "user", "key"})
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

func TestHostTrustUsesSavedHost(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{
		Name:           "devhost",
		IP:             "10.0.0.8",
		User:           "root",
		KeyPath:        "~/.ssh/id_rsa",
		Port:           2222,
		KnownHostsPath: "~/.ssh/custom_known_hosts",
	}}}); err != nil {
		t.Fatal(err)
	}

	var gotHost core.Host
	t.Cleanup(setHostTrustForTest(func(host core.Host, opts core.HostKeyTrustOptions) (core.HostKeyTrustResult, error) {
		gotHost = host
		if opts.Force {
			t.Fatal("force = true, want false")
		}
		return core.HostKeyTrustResult{
			Address:        "10.0.0.8:2222",
			KnownHostsPath: "~/.ssh/custom_known_hosts",
			KeyType:        "ssh-ed25519",
			Fingerprint:    "SHA256:test",
			Status:         "added",
		}, nil
	}))

	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "trust", "devhost"}); err != nil {
		t.Fatalf("host trust: %v", err)
	}
	if gotHost.IP != "10.0.0.8" || gotHost.Port != 2222 || gotHost.KnownHostsPath != "~/.ssh/custom_known_hosts" {
		t.Fatalf("host = %+v", gotHost)
	}
	if !strings.Contains(out.String(), "trusted host key: 10.0.0.8:2222 ssh-ed25519 SHA256:test") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestHostTrustForcePassesOption(t *testing.T) {
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

	var gotForce bool
	t.Cleanup(setHostTrustForTest(func(host core.Host, opts core.HostKeyTrustOptions) (core.HostKeyTrustResult, error) {
		gotForce = opts.Force
		return core.HostKeyTrustResult{
			Address:        "10.0.0.8:22",
			KnownHostsPath: "~/.ssh/known_hosts",
			KeyType:        "ssh-ed25519",
			Fingerprint:    "SHA256:test",
			Status:         "replaced",
		}, nil
	}))

	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "trust", "-f", "devhost"}); err != nil {
		t.Fatalf("host trust force: %v", err)
	}
	if !gotForce {
		t.Fatal("force = false, want true")
	}
	if !strings.Contains(out.String(), "replaced host key: 10.0.0.8:22 ssh-ed25519 SHA256:test") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestHostTrustRawTargetWithPort(t *testing.T) {
	withTempConfig(t)
	var gotHost core.Host
	t.Cleanup(setHostTrustForTest(func(host core.Host, opts core.HostKeyTrustOptions) (core.HostKeyTrustResult, error) {
		gotHost = host
		if opts.Force {
			t.Fatal("force = true, want false")
		}
		return core.HostKeyTrustResult{
			Address:        "192.168.1.10:2222",
			KnownHostsPath: "~/.ssh/known_hosts",
			KeyType:        "ssh-rsa",
			Fingerprint:    "SHA256:raw",
			Status:         "already_trusted",
		}, nil
	}))

	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "trust", "192.168.1.10", "--port", "2222"}); err != nil {
		t.Fatalf("host trust raw: %v", err)
	}
	if gotHost.IP != "192.168.1.10" || gotHost.Port != 2222 {
		t.Fatalf("host = %+v", gotHost)
	}
	if !strings.Contains(out.String(), "host key already trusted: 192.168.1.10:2222 ssh-rsa SHA256:raw") {
		t.Fatalf("output = %q", out.String())
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
	err := app.RunWithArgs([]string{"host", "set", "devhost", "ip=10.0.0.9"})
	if err == nil || !strings.Contains(err.Error(), "host ip") {
		t.Fatalf("err = %v", err)
	}
}

func TestHostSetRejectsInvalidFieldWithoutSaving(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{
		Name:    "devhost",
		IP:      "10.0.0.8",
		User:    "root",
		KeyPath: "~/.ssh/id_rsa",
		Remark:  "old",
		Port:    22,
	}}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	err := app.RunWithArgs([]string{"host", "set", "devhost", "remark=new", "bad=value"})
	if err == nil || !strings.Contains(err.Error(), "unknown host field") {
		t.Fatalf("err = %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Hosts[0].Remark != "old" {
		t.Fatalf("host was changed: %+v", config.Hosts[0])
	}
}
