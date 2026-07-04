package command

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/gcli/v3"
)

func TestAddAndList(t *testing.T) {
	withTempConfig(t)

	app := newTestApp()
	err := app.RunWithArgs([]string{
		"add",
		"--ip", "10.0.0.8",
		"-u", "root",
		"-p", "secret",
		"--name", "devhost",
		"--key", "~/.ssh/id_rsa",
		"--remark", "testing host",
		"--group", "testing",
	})
	if err != nil {
		t.Fatalf("add host: %v", err)
	}

	store := readTestStore(t)
	if len(store.Hosts) != 1 {
		t.Fatalf("hosts len = %d, want 1", len(store.Hosts))
	}
	if store.Hosts[0].Name != "devhost" || store.Hosts[0].IP != "10.0.0.8" || store.Hosts[0].User != "root" {
		t.Fatalf("unexpected host: %+v", store.Hosts[0])
	}
	if store.Hosts[0].KeyPath != "~/.ssh/id_rsa" || store.Hosts[0].Remark != "testing host" || store.Hosts[0].Group != "testing" {
		t.Fatalf("unexpected host metadata: %+v", store.Hosts[0])
	}
}

func TestAddAllowsKeyPathWithoutPassword(t *testing.T) {
	withTempConfig(t)

	app := newTestApp()
	err := app.RunWithArgs([]string{"add", "--ip", "10.0.0.8", "-u", "root", "--name", "devhost", "--key", "~/.ssh/id_rsa"})
	if err != nil {
		t.Fatalf("add host with key: %v", err)
	}

	store := readTestStore(t)
	if len(store.Hosts) != 1 {
		t.Fatalf("hosts len = %d, want 1", len(store.Hosts))
	}
	if store.Hosts[0].Password != "" || store.Hosts[0].KeyPath != "~/.ssh/id_rsa" {
		t.Fatalf("unexpected auth fields: %+v", store.Hosts[0])
	}
}

func TestAddFromClipboard(t *testing.T) {
	withTempConfig(t)
	t.Cleanup(setReadClipboardForTest(func() (string, error) {
		return "ip=10.0.0.8\nuser=root\nkey=~/.ssh/id_rsa\nname=devhost\nremark=testing host\ngroup=testing\n", nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"add", "--from-clipboard"}); err != nil {
		t.Fatalf("add from clipboard: %v", err)
	}
	store := readTestStore(t)
	if len(store.Hosts) != 1 {
		t.Fatalf("hosts len = %d, want 1", len(store.Hosts))
	}
	host := store.Hosts[0]
	if host.Name != "devhost" || host.IP != "10.0.0.8" || host.User != "root" || host.KeyPath != "~/.ssh/id_rsa" {
		t.Fatalf("host = %+v", host)
	}
}

func TestParseClipboardHostCSV(t *testing.T) {
	host, err := parseClipboardHost("10.0.0.8,root,secret,devhost,2222")
	if err != nil {
		t.Fatal(err)
	}
	if host.Name != "devhost" || host.IP != "10.0.0.8" || host.User != "root" || host.Password != "secret" || host.Port != 2222 {
		t.Fatalf("host = %+v", host)
	}
}

func TestParseClipboardHostErrors(t *testing.T) {
	if _, err := parseClipboardHost(""); err == nil {
		t.Fatal("expected empty clipboard error")
	}
	if _, err := parseClipboardHost("only,two"); err == nil {
		t.Fatal("expected CSV format error")
	}
}

func TestCollectInteractiveHost(t *testing.T) {
	input := strings.NewReader("devhost\n10.0.0.8\nroot\n\n~/.ssh/id_rsa\n2222\ntesting host\ntesting\n")
	host, err := collectInteractiveHost(input, &strings.Builder{})
	if err != nil {
		t.Fatalf("collect interactive host: %v", err)
	}
	if host.Name != "devhost" || host.IP != "10.0.0.8" || host.User != "root" || host.Port != 2222 {
		t.Fatalf("host = %+v", host)
	}
	if host.Password != "" || host.KeyPath != "~/.ssh/id_rsa" || host.Remark != "testing host" || host.Group != "testing" {
		t.Fatalf("host metadata = %+v", host)
	}
}

func TestCollectInteractiveHostDefaults(t *testing.T) {
	input := strings.NewReader("\n10.0.0.8\n\nsecret\n\n\n\n\n")
	host, err := collectInteractiveHost(input, &strings.Builder{})
	if err != nil {
		t.Fatalf("collect interactive host: %v", err)
	}
	if host.Name != "10.0.0.8" || host.User != "root" || host.Port != core.DefaultSSHPort || host.Group != core.DefaultGroup {
		t.Fatalf("host = %+v", host)
	}
}

func TestPromptPasswordUsesHiddenReader(t *testing.T) {
	var gotQuestion string
	t.Cleanup(setReadInteractivePasswordForTest(func(question ...string) string {
		if len(question) > 0 {
			gotQuestion = question[0]
		}
		return " secret "
	}))

	password, err := promptPassword(bufio.NewReader(strings.NewReader("")), &strings.Builder{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if password != "secret" {
		t.Fatalf("password = %q, want secret", password)
	}
	if gotQuestion != "Password: " {
		t.Fatalf("question = %q, want Password: ", gotQuestion)
	}
}

func TestBuildHostListTable(t *testing.T) {
	hosts := []core.Host{{
		Name:    "devhost",
		IP:      "10.0.0.8",
		User:    "root",
		KeyPath: "~/.ssh/id_rsa",
		Remark:  "testing host",
		Group:   "testing",
		Port:    2222,
	}}

	out := buildHostListTable(hosts, false)
	for _, want := range []string{"Name", "Group", "Address", "Auth", "Remark", "devhost", "testing", "root@10.*.*.8:2222", "key", "testing host"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table output %q does not contain %q", out, want)
		}
	}
	if strings.Contains(out, "10.0.0.8") {
		t.Fatalf("table output %q should mask IPv4 address", out)
	}

	fullOut := buildHostListTable(hosts, true)
	if !strings.Contains(fullOut, "root@10.0.0.8:2222") {
		t.Fatalf("table output %q does not contain full IP", fullOut)
	}
}

func TestListCommandMasksIPByDefault(t *testing.T) {
	withTempConfig(t)
	store := &core.Store{Hosts: []core.Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     2222,
	}}}
	if err := core.SaveStore(store); err != nil {
		t.Fatalf("save store: %v", err)
	}
	t.Cleanup(func() { listOpts.ShowIP = false })

	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	if err := app.RunWithArgs([]string{"list"}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.String(), "root@10.*.*.8:2222") || strings.Contains(out.String(), "root@10.0.0.8:2222") {
		t.Fatalf("masked list output = %q", out.String())
	}

	out.Reset()
	if err := app.RunWithArgs([]string{"list", "--show-ip"}); err != nil {
		t.Fatalf("list --show-ip: %v", err)
	}
	if !strings.Contains(out.String(), "root@10.0.0.8:2222") {
		t.Fatalf("full list output = %q", out.String())
	}
}

func TestHostAddWithAuthRef(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "add", "--ip", "10.0.0.8", "--name", "devhost", "--auth", "dev-root"}); err != nil {
		t.Fatalf("host add: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AuthProfiles) != 1 {
		t.Fatalf("auth profiles were not preserved: %+v", config.AuthProfiles)
	}
	if len(config.Hosts) != 1 || config.Hosts[0].AuthRef != "dev-root" {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestTopLevelAddStillPreservesConfig(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"add", "--ip", "10.0.0.8", "--name", "devhost", "--auth", "dev-root"}); err != nil {
		t.Fatalf("top-level add: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.AuthProfiles) != 1 || len(config.Hosts) != 1 || config.Hosts[0].AuthRef != "dev-root" {
		t.Fatalf("config = %+v", config)
	}
}

func TestHostListFiltersByGroupAndMatch(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{
		{Name: "testing-web", IP: "10.0.0.8", User: "root", Password: "secret", Group: "testing", Port: 22},
		{Name: "prod-db", IP: "10.0.0.9", User: "root", Password: "secret", Group: "prod", Port: 22},
	}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	if err := app.RunWithArgs([]string{"host", "list", "--group", "testing", "--match", "web"}); err != nil {
		t.Fatalf("host list: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "testing-web") || strings.Contains(output, "prod-db") {
		t.Fatalf("output = %q", output)
	}
}

func TestHostShowMasksSecrets(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	if err := app.RunWithArgs([]string{"host", "show", "devhost"}); err != nil {
		t.Fatalf("host show: %v", err)
	}
	if strings.Contains(out.String(), "secret") || !strings.Contains(out.String(), `"password_enc": "***"`) {
		t.Fatalf("output = %q", out.String())
	}
}

func TestHostRemoveRequiresYesInNonInteractiveTest(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22}}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	err := app.RunWithArgs([]string{"host", "rm", "devhost"})
	if err == nil || !strings.Contains(err.Error(), "use --yes") {
		t.Fatalf("err = %v", err)
	}
}

func TestHostRemoveWithYesAndRename(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{
		{Name: "old-name", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22},
		{Name: "delete-me", IP: "10.0.0.9", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22},
	}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"host", "rename", "old-name", "new-name"}); err != nil {
		t.Fatalf("host rename: %v", err)
	}
	if err := app.RunWithArgs([]string{"host", "rm", "delete-me", "--yes"}); err != nil {
		t.Fatalf("host rm: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Hosts) != 1 || config.Hosts[0].Name != "new-name" {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestDisplayHostIP(t *testing.T) {
	tests := []struct {
		name   string
		ip     string
		showIP bool
		want   string
	}{
		{name: "masked ipv4", ip: "10.0.0.8", want: "10.*.*.8"},
		{name: "full ipv4", ip: "10.0.0.8", showIP: true, want: "10.0.0.8"},
		{name: "hostname", ip: "devhost.local", want: "devhost.local"},
		{name: "ipv6", ip: "2001:db8::1", want: "2001:db8::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayHostIP(tt.ip, tt.showIP); got != tt.want {
				t.Fatalf("displayHostIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoginUsesSavedHostAndWritesLog(t *testing.T) {
	withTempConfig(t)
	store := &core.Store{Hosts: []core.Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     2222,
	}}}
	if err := core.SaveStore(store); err != nil {
		t.Fatalf("save store: %v", err)
	}

	var gotHost core.Host
	var gotOpts core.LoginOptions
	t.Cleanup(setLoginRemoteForTest(func(host core.Host, opts core.LoginOptions) error {
		gotHost = host
		gotOpts = opts
		return nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"connect", "--term", "vt100", "devhost"}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if gotHost.IP != "10.0.0.8" {
		t.Fatalf("host ip = %q", gotHost.IP)
	}
	if gotOpts.Term != "vt100" {
		t.Fatalf("term = %q, want vt100", gotOpts.Term)
	}
	lines, err := core.ReadRunLogs("devhost", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], `"command":"login"`) {
		t.Fatalf("logs = %#v", lines)
	}
}

func TestResolveLogTargetUsesSavedHost(t *testing.T) {
	withTempConfig(t)
	store := &core.Store{Hosts: []core.Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     22,
	}}}
	if err := core.SaveStore(store); err != nil {
		t.Fatalf("save store: %v", err)
	}

	target, err := resolveLogTarget("10.0.0.8")
	if err != nil {
		t.Fatal(err)
	}
	if target != "devhost" {
		t.Fatalf("target = %q, want devhost", target)
	}
}

type testApp struct {
	*gcli.App
	lastErr error
}

func (a *testApp) RunWithArgs(args []string) error {
	a.lastErr = nil
	if code := a.Run(args); code != 0 && a.lastErr == nil {
		return errors.New("command failed")
	}
	return a.lastErr
}

func newTestApp() *testApp {
	app := gcli.NewApp()
	app.Name = "sshc"
	app.Desc = "simple ssh command runner"
	ta := &testApp{App: app}
	app.On(gcli.EvtCmdRunError, func(ctx *gcli.HookCtx) bool {
		if err, ok := ctx.Data["err"].(error); ok {
			ta.lastErr = err
		}
		return false
	})
	app.On(gcli.EvtAppRunError, func(ctx *gcli.HookCtx) bool {
		if err, ok := ctx.Data["err"].(error); ok {
			ta.lastErr = err
		}
		return false
	})
	app.Add(NewAddCmd(), NewAuthCmd(), NewCfgCmd(), NewHostCmd(), NewRunCmd(), NewBatchRunCmd(), NewUploadCmd(), NewDownloadCmd(), NewListCmd(), NewLogCmd(), NewLoginCmd())
	return ta
}

func withTempConfig(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(core.SetUserHomeDirForTest(func() (string, error) { return home, nil }))

	path := filepath.Join(home, core.ConfigFileName)
	t.Setenv(core.ConfigEnvKey, path)
	return path
}

func saveJumpCommandHosts(t *testing.T) {
	t.Helper()
	withTempConfig(t)
	config := &core.Config{
		Defaults: core.Defaults{HostKeyCheck: core.HostKeyCheckInsecure},
		AuthProfiles: []core.AuthProfile{
			{Name: "ops", User: "root", Password: "secret"},
		},
		Hosts: []core.Host{
			{Name: "bastion", IP: "1.2.3.4", AuthRef: "ops"},
			{Name: "alt-bastion", IP: "1.2.3.5", AuthRef: "ops"},
			{Name: "inner-db", IP: "10.0.0.8", AuthRef: "ops", Jump: "bastion"},
		},
	}
	if err := core.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
}

func readTestStore(t *testing.T) core.Store {
	t.Helper()
	path, err := core.StorePath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var store core.Store
	if err := json.Unmarshal(data, &store); err != nil {
		t.Fatal(err)
	}
	return store
}
