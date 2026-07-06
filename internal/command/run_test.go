package command

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inhere/sshc/internal/core"
)

func TestRunUsesSavedHost(t *testing.T) {
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
	var gotCommand string
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotHost = host
		gotCommand = command
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "devhost", "--", "echo", "hello"}); err != nil {
		t.Fatalf("run host: %v", err)
	}
	if gotHost.IP != "10.0.0.8" {
		t.Fatalf("host ip = %q", gotHost.IP)
	}
	if gotCommand != "echo hello" {
		t.Fatalf("command = %q", gotCommand)
	}

	lines, err := core.ReadRunLogs("devhost", "", 10)
	if err != nil {
		t.Fatalf("read run logs: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("log lines len = %d, want 1", len(lines))
	}
	line := lines[0]
	for _, want := range []string{`"command":"echo hello"`, `"status":"success"`, `"output":"ok\n"`} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q does not contain %q", line, want)
		}
	}
}

func TestRunCommandProxyWritesLogFields(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{
		{Name: "pve-host", IP: "192.168.1.20", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22, HostKeyCheck: core.HostKeyCheckInsecure},
		{Name: "lxc-app", Backend: core.HostBackendCommandProxy, Via: "pve-host", RunTemplate: "pct exec 101 -- sh -lc {{cmd}}"},
	}}); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		if host.Name != "lxc-app" || command != "hostname" {
			t.Fatalf("host=%+v command=%q", host, command)
		}
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "lxc-app", "--", "hostname"}); err != nil {
		t.Fatalf("run command_proxy: %v", err)
	}
	lines, err := core.ReadRunLogs("lxc-app", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("logs = %#v", lines)
	}
	for _, want := range []string{`"backend":"command_proxy"`, `"via":"pve-host"`, `"proxied_command":"pct exec 101 -- sh -lc 'hostname'"`} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("log line %q does not contain %q", lines[0], want)
		}
	}
}

func TestRunCommandProxyRejectsScript(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Hosts: []core.Host{
		{Name: "pve-host", IP: "192.168.1.20", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22, HostKeyCheck: core.HostKeyCheckInsecure},
		{Name: "lxc-app", Backend: core.HostBackendCommandProxy, Via: "pve-host", RunTemplate: "pct exec 101 -- sh -lc {{cmd}}"},
	}}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(t.TempDir(), "deploy.sh")
	if err := os.WriteFile(scriptPath, []byte("echo ok\n"), 0600); err != nil {
		t.Fatal(err)
	}
	app := newTestApp()
	err := app.RunWithArgs([]string{"run", "--script", scriptPath, "lxc-app"})
	if err == nil || !strings.Contains(err.Error(), "--script is not supported") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunUsesPartialHostTarget(t *testing.T) {
	withTempConfig(t)
	store := &core.Store{Hosts: []core.Host{{
		Name:     "testing-web",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     2222,
	}}}
	if err := core.SaveStore(store); err != nil {
		t.Fatalf("save store: %v", err)
	}

	var gotHost core.Host
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotHost = host
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "test web", "--", "hostname"}); err != nil {
		t.Fatalf("run host: %v", err)
	}
	if gotHost.Name != "testing-web" {
		t.Fatalf("host = %+v", gotHost)
	}
}

func TestRunRejectsAmbiguousPartialHostTarget(t *testing.T) {
	withTempConfig(t)
	store := &core.Store{Hosts: []core.Host{
		{Name: "testing-web", IP: "10.0.0.8", User: "root", Password: "secret", Port: 2222},
		{Name: "testing-db", IP: "10.0.0.9", User: "root", Password: "secret", Port: 2222},
	}}
	if err := core.SaveStore(store); err != nil {
		t.Fatalf("save store: %v", err)
	}

	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		t.Fatal("run should not be called")
		return nil, nil
	}))

	app := newTestApp()
	err := app.RunWithArgs([]string{"run", "testing", "--", "hostname"})
	if err == nil {
		t.Fatal("expected ambiguous target error")
	}
	if !strings.Contains(err.Error(), "matches multiple hosts") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunPassesTimeoutAndEnvOptions(t *testing.T) {
	withTempConfig(t)
	envFile := filepath.Join(t.TempDir(), "run.env")
	if err := os.WriteFile(envFile, []byte("FOO=file\nBAR=bar\n"), 0600); err != nil {
		t.Fatal(err)
	}
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

	var gotOpts core.RunOptions
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotOpts = opts
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	err := app.RunWithArgs([]string{
		"run",
		"--timeout", "3s",
		"--kill-after", "5",
		"--efile", envFile,
		"-e", "FOO=inline",
		"-e", "BAZ=baz",
		"devhost",
		"--", "printenv", "FOO",
	})
	if err != nil {
		t.Fatalf("run host: %v", err)
	}
	if gotOpts.Timeout != 3*time.Second {
		t.Fatalf("timeout = %s", gotOpts.Timeout)
	}
	if gotOpts.KillAfter != 5*time.Second {
		t.Fatalf("kill after = %s", gotOpts.KillAfter)
	}
	if gotOpts.Env["FOO"] != "inline" || gotOpts.Env["BAR"] != "bar" || gotOpts.Env["BAZ"] != "baz" {
		t.Fatalf("env = %#v", gotOpts.Env)
	}
}

func TestRunUsesEffectiveHostDefaultsAndAuthRef(t *testing.T) {
	withTempConfig(t)
	config := &core.Config{
		Defaults: core.Defaults{
			User:            "root",
			Port:            2200,
			RunTimeout:      "7s",
			RemoteScriptDir: "/opt/app/tmp",
			HostKeyCheck:    core.HostKeyCheckInsecure,
		},
		AuthProfiles: []core.AuthProfile{{
			Name:     "dev-root",
			Password: "secret",
		}},
		Hosts: []core.Host{{
			Name:    "devhost",
			IP:      "10.0.0.8",
			AuthRef: "dev-root",
		}},
	}
	if err := core.SaveConfig(config); err != nil {
		t.Fatalf("save config: %v", err)
	}

	var gotHost core.Host
	var gotOpts core.RunOptions
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotHost = host
		gotOpts = opts
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "devhost", "--", "uptime"}); err != nil {
		t.Fatalf("run host: %v", err)
	}
	if gotHost.User != "root" || gotHost.Password != "secret" || gotHost.Port != 2200 {
		t.Fatalf("host = %+v", gotHost)
	}
	if gotHost.HostKeyCheck != core.HostKeyCheckInsecure {
		t.Fatalf("host key check = %q", gotHost.HostKeyCheck)
	}
	if gotOpts.Timeout != 7*time.Second || gotOpts.RemoteScriptDir != "/opt/app/tmp" {
		t.Fatalf("opts = %+v", gotOpts)
	}
}

func TestRunPassesJumpOption(t *testing.T) {
	saveJumpCommandHosts(t)

	var gotHost core.Host
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotHost = host
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "inner-db", "--jump", "bastion", "--", "hostname"}); err != nil {
		t.Fatalf("run with jump: %v", err)
	}
	if gotHost.Jump != "bastion" {
		t.Fatalf("jump = %q, want bastion", gotHost.Jump)
	}
}

func TestRunUsesConfiguredJump(t *testing.T) {
	saveJumpCommandHosts(t)

	var gotHost core.Host
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotHost = host
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "inner-db", "--", "hostname"}); err != nil {
		t.Fatalf("run with configured jump: %v", err)
	}
	if gotHost.Jump != "bastion" {
		t.Fatalf("jump = %q, want bastion", gotHost.Jump)
	}
}

func TestRunJumpOptionOverridesConfiguredJump(t *testing.T) {
	saveJumpCommandHosts(t)

	var gotHost core.Host
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotHost = host
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "--jump", "alt-bastion", "inner-db", "--", "hostname"}); err != nil {
		t.Fatalf("run with jump override: %v", err)
	}
	if gotHost.Jump != "alt-bastion" {
		t.Fatalf("jump = %q, want alt-bastion", gotHost.Jump)
	}
}

func TestRunParsesJumpBeforeTarget(t *testing.T) {
	saveJumpCommandHosts(t)

	var gotHost core.Host
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotHost = host
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "--jump", "bastion", "inner-db", "--", "hostname"}); err != nil {
		t.Fatalf("run with jump before target: %v", err)
	}
	if gotHost.Jump != "bastion" {
		t.Fatalf("jump = %q, want bastion", gotHost.Jump)
	}
}

func TestRunParsesJumpAfterTargetBeforeCommand(t *testing.T) {
	saveJumpCommandHosts(t)

	var gotHost core.Host
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotHost = host
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "inner-db", "--jump", "bastion", "--", "hostname"}); err != nil {
		t.Fatalf("run with jump after target: %v", err)
	}
	if gotHost.Jump != "bastion" {
		t.Fatalf("jump = %q, want bastion", gotHost.Jump)
	}
}

func TestRunPassesCWDOption(t *testing.T) {
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

	var gotOpts core.RunOptions
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotOpts = opts
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "--cwd", "/opt/app", "devhost", "--", "pwd"}); err != nil {
		t.Fatalf("run host: %v", err)
	}
	if gotOpts.CWD != "/opt/app" {
		t.Fatalf("cwd = %q, want /opt/app", gotOpts.CWD)
	}

	lines, err := core.ReadRunLogs("devhost", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], `"cwd":"/opt/app"`) {
		t.Fatalf("logs = %#v", lines)
	}
}

func TestRunPassesSudoOptions(t *testing.T) {
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

	var gotOpts core.RunOptions
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotOpts = opts
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "--sudo-user", "ylpy", "devhost", "--", "whoami"}); err != nil {
		t.Fatalf("run host: %v", err)
	}
	if gotOpts.SudoUser != "ylpy" {
		t.Fatalf("sudo user = %q, want ylpy", gotOpts.SudoUser)
	}
}

func TestRunRejectsConflictingSudoOptions(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	err := app.RunWithArgs([]string{"run", "--sudo", "--sudo-user", "ylpy", "devhost", "--", "whoami"})
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunPassesScriptOptions(t *testing.T) {
	withTempConfig(t)
	scriptPath := filepath.Join(t.TempDir(), "deploy.sh")
	if err := os.WriteFile(scriptPath, []byte("echo ok\n"), 0600); err != nil {
		t.Fatal(err)
	}
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

	var gotCommand string
	var gotOpts core.RunOptions
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotCommand = command
		gotOpts = opts
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "--script", scriptPath, "--keep-remote-script", "devhost"}); err != nil {
		t.Fatalf("run script: %v", err)
	}
	if gotCommand != "" {
		t.Fatalf("command = %q, want empty", gotCommand)
	}
	if gotOpts.ScriptPath != scriptPath {
		t.Fatalf("script = %q, want %q", gotOpts.ScriptPath, scriptPath)
	}
	if gotOpts.RemoteScriptPath == "" || !strings.HasPrefix(gotOpts.RemoteScriptPath, "/tmp/sshc-run-") {
		t.Fatalf("remote script = %q", gotOpts.RemoteScriptPath)
	}
	if !gotOpts.KeepRemoteScript {
		t.Fatal("keep remote script = false, want true")
	}

	lines, err := core.ReadRunLogs("devhost", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("log lines len = %d, want 1", len(lines))
	}
	line := lines[0]
	for _, want := range []string{`"script":"` + strings.ReplaceAll(scriptPath, `\`, `\\`) + `"`, `"keep_remote_script":true`, `"remote_script":"/tmp/sshc-run-`} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q does not contain %q", line, want)
		}
	}
}

func TestRunPassesRemoteScriptDir(t *testing.T) {
	withTempConfig(t)
	scriptPath := filepath.Join(t.TempDir(), "deploy.sh")
	if err := os.WriteFile(scriptPath, []byte("echo ok\n"), 0600); err != nil {
		t.Fatal(err)
	}
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

	var gotOpts core.RunOptions
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotOpts = opts
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"run", "--script", scriptPath, "--remote-script-dir", "/opt/app/tmp", "devhost"}); err != nil {
		t.Fatalf("run script: %v", err)
	}
	if gotOpts.RemoteScriptDir != "/opt/app/tmp" {
		t.Fatalf("remote script dir = %q, want /opt/app/tmp", gotOpts.RemoteScriptDir)
	}
	if !strings.HasPrefix(gotOpts.RemoteScriptPath, "/opt/app/tmp/sshc-run-") {
		t.Fatalf("remote script = %q", gotOpts.RemoteScriptPath)
	}
}

func TestRunPrintsScriptFailureContext(t *testing.T) {
	withTempConfig(t)
	scriptPath := filepath.Join(t.TempDir(), "deploy.sh")
	if err := os.WriteFile(scriptPath, []byte("echo ok\n"), 0600); err != nil {
		t.Fatal(err)
	}
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

	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		return []byte("permission denied\n"), errors.New("exit status 126")
	}))

	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	err := app.RunWithArgs([]string{"run", "--script", scriptPath, "--sudo-user", "app", "devhost"})
	if err == nil {
		t.Fatal("expected run error")
	}
	output := out.String()
	for _, want := range []string{
		"permission denied",
		"sshc: local_script=" + scriptPath,
		"sshc: remote_script=/tmp/sshc-run-",
		"sshc: sudo_user=app",
		"sshc: use --keep-remote-script to inspect the uploaded script",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
}

func TestRunDoesNotPrintScriptFailureContextForCommandFailure(t *testing.T) {
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

	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		return []byte("failed\n"), errors.New("exit status 1")
	}))

	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	err := app.RunWithArgs([]string{"run", "devhost", "--", "hostname"})
	if err == nil {
		t.Fatal("expected run error")
	}
	if strings.Contains(out.String(), "sshc: local_script=") {
		t.Fatalf("unexpected script failure context: %q", out.String())
	}
}

func TestRunRejectsCommandAndScriptTogether(t *testing.T) {
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

	app := newTestApp()
	err := app.RunWithArgs([]string{"run", "--script", "deploy.sh", "devhost", "--", "hostname"})
	if err == nil || !strings.Contains(err.Error(), "cannot be used together") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunRejectsRemoteScriptDirWithoutScript(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	err := app.RunWithArgs([]string{"run", "--remote-script-dir", "/opt/app/tmp", "devhost", "--", "hostname"})
	if err == nil || !strings.Contains(err.Error(), "--remote-script-dir requires --script") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunRequiresCommandOrScript(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	err := app.RunWithArgs([]string{"run", "devhost"})
	if err == nil || !strings.Contains(err.Error(), "remote command or --script is required") {
		t.Fatalf("err = %v", err)
	}
}
