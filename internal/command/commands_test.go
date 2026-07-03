package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/goutil/cflag/capp"
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
	app.BeforeRun = func(c *capp.Cmd, cmdArgs []string) bool {
		if c.Name == "run" {
			c.SetOutput(&out)
		}
		return true
	}
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
	app.BeforeRun = func(c *capp.Cmd, cmdArgs []string) bool {
		if c.Name == "run" {
			c.SetOutput(&out)
		}
		return true
	}
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

func TestSCPUsesSavedHost(t *testing.T) {
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
	var gotJobs []core.TransferJob
	var gotOpts core.TransferOptions
	t.Cleanup(setUploadRemoteForTest(func(host core.Host, jobs []core.TransferJob, opts core.TransferOptions) (core.TransferResult, error) {
		gotHost = host
		gotJobs = jobs
		gotOpts = opts
		return core.TransferResult{Bytes: 123, Files: 1, Directories: 0, Elapsed: 1500 * time.Millisecond}, nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"scp", "--sha256", "--remove-dir", "-l", "local.txt", "-r", "/tmp/remote.txt", "devhost"}); err != nil {
		t.Fatalf("scp: %v", err)
	}
	if gotHost.IP != "10.0.0.8" {
		t.Fatalf("host ip = %q", gotHost.IP)
	}
	if len(gotJobs) != 1 || gotJobs[0].LocalPath != "local.txt" || gotJobs[0].RemotePath != "/tmp/remote.txt" {
		t.Fatalf("jobs = %+v", gotJobs)
	}
	if !gotOpts.SHA256 {
		t.Fatal("sha256 option = false, want true")
	}
	if !gotOpts.RemoveDir {
		t.Fatal("remove-dir option = false, want true")
	}
}

func TestSCPUsesRepeatedLocalPaths(t *testing.T) {
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

	var gotJobs []core.TransferJob
	t.Cleanup(setUploadRemoteForTest(func(host core.Host, jobs []core.TransferJob, opts core.TransferOptions) (core.TransferResult, error) {
		gotJobs = jobs
		return core.TransferResult{Bytes: 2, Files: 2, Elapsed: time.Second}, nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"upload", "-l", "a.jar", "-l", "b.jar", "-r", "/opt/app/lib", "devhost"}); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if len(gotJobs) != 2 {
		t.Fatalf("jobs len = %d, want 2: %+v", len(gotJobs), gotJobs)
	}
	for i, want := range []string{"a.jar", "b.jar"} {
		if gotJobs[i].LocalPath != want || gotJobs[i].RemotePath != "/opt/app/lib" || !gotJobs[i].RemoteDir {
			t.Fatalf("job[%d] = %+v", i, gotJobs[i])
		}
	}
}

func TestSCPUsesUploadMaps(t *testing.T) {
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

	var gotJobs []core.TransferJob
	t.Cleanup(setUploadRemoteForTest(func(host core.Host, jobs []core.TransferJob, opts core.TransferOptions) (core.TransferResult, error) {
		gotJobs = jobs
		return core.TransferResult{Bytes: 2, Files: 2, Elapsed: time.Second}, nil
	}))

	app := newTestApp()
	err := app.RunWithArgs([]string{
		"upload",
		"--map", "./config/app.yml=/etc/app/app.yml",
		"--map", "./scripts/deploy.sh=/opt/app/deploy.sh",
		"devhost",
	})
	if err != nil {
		t.Fatalf("upload map: %v", err)
	}
	if len(gotJobs) != 2 {
		t.Fatalf("jobs len = %d, want 2: %+v", len(gotJobs), gotJobs)
	}
	if gotJobs[0].LocalPath != "./config/app.yml" || gotJobs[0].RemotePath != "/etc/app/app.yml" || gotJobs[0].RemoteDir {
		t.Fatalf("job[0] = %+v", gotJobs[0])
	}
	if gotJobs[1].LocalPath != "./scripts/deploy.sh" || gotJobs[1].RemotePath != "/opt/app/deploy.sh" || gotJobs[1].RemoteDir {
		t.Fatalf("job[1] = %+v", gotJobs[1])
	}
}

func TestSCPRejectsInvalidMultiPathOptions(t *testing.T) {
	withTempConfig(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "map with local",
			args: []string{"upload", "--map", "a=b", "-l", "a", "devhost"},
			want: "--map cannot be used with --local or --remote",
		},
		{
			name: "map with remove dir",
			args: []string{"upload", "--map", "a=b", "--remove-dir", "devhost"},
			want: "--remove-dir cannot be used with --map",
		},
		{
			name: "invalid map",
			args: []string{"upload", "--map", "a", "devhost"},
			want: "invalid --map",
		},
		{
			name: "glob map",
			args: []string{"upload", "--map", "*.jar=/opt/app/lib", "devhost"},
			want: "--map does not support local glob",
		},
		{
			name: "remove dir with repeated local",
			args: []string{"upload", "-l", "a", "-l", "b", "-r", "/opt/app", "--remove-dir", "devhost"},
			want: "--remove-dir is only supported for a single directory upload",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApp()
			err := app.RunWithArgs(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSCPRequiresSavedHost(t *testing.T) {
	withTempConfig(t)
	t.Cleanup(setUploadRemoteForTest(func(host core.Host, jobs []core.TransferJob, opts core.TransferOptions) (core.TransferResult, error) {
		t.Fatal("upload should not be called")
		return core.TransferResult{}, nil
	}))

	app := newTestApp()
	err := app.RunWithArgs([]string{"scp", "-l", "local.txt", "-r", "/tmp/remote.txt", "missing"})
	if err == nil || !strings.Contains(err.Error(), `host "missing" not found`) {
		t.Fatalf("err = %v", err)
	}
}

func TestFormatElapsedRoundsToMilliseconds(t *testing.T) {
	if got := formatElapsed(1500*time.Millisecond + 499*time.Microsecond); got != "1.5s" {
		t.Fatalf("elapsed = %q, want 1.5s", got)
	}
	if got := formatElapsed(-time.Second); got != "0s" {
		t.Fatalf("elapsed = %q, want 0s", got)
	}
}

func TestDownloadUsesSavedHost(t *testing.T) {
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
	var gotRemote string
	var gotLocal string
	var gotOpts core.TransferOptions
	t.Cleanup(setDownloadRemoteForTest(func(host core.Host, remotePath, localPath string, opts core.TransferOptions) (core.TransferResult, error) {
		gotHost = host
		gotRemote = remotePath
		gotLocal = localPath
		gotOpts = opts
		return core.TransferResult{Bytes: 456, Files: 2, Directories: 1, Elapsed: 2 * time.Second}, nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"download", "--sha256", "-r", "/tmp/remote.txt", "-l", "local.txt", "devhost"}); err != nil {
		t.Fatalf("download: %v", err)
	}
	if gotHost.IP != "10.0.0.8" {
		t.Fatalf("host ip = %q", gotHost.IP)
	}
	if gotRemote != "/tmp/remote.txt" || gotLocal != "local.txt" {
		t.Fatalf("paths = %q -> %q", gotRemote, gotLocal)
	}
	if !gotOpts.SHA256 {
		t.Fatal("sha256 option = false, want true")
	}
}

func TestDownloadAlias(t *testing.T) {
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

	t.Cleanup(setDownloadRemoteForTest(func(host core.Host, remotePath, localPath string, opts core.TransferOptions) (core.TransferResult, error) {
		return core.TransferResult{}, nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"dl", "-r", "/tmp/remote.txt", "-l", "local.txt", "devhost"}); err != nil {
		t.Fatalf("dl: %v", err)
	}
}

func TestDownloadRequiresSavedHost(t *testing.T) {
	withTempConfig(t)
	t.Cleanup(setDownloadRemoteForTest(func(host core.Host, remotePath, localPath string, opts core.TransferOptions) (core.TransferResult, error) {
		t.Fatal("download should not be called")
		return core.TransferResult{}, nil
	}))

	app := newTestApp()
	err := app.RunWithArgs([]string{"download", "-r", "/tmp/remote.txt", "-l", "local.txt", "missing"})
	if err == nil || !strings.Contains(err.Error(), `host "missing" not found`) {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildHostListTable(t *testing.T) {
	out := buildHostListTable([]core.Host{{
		Name:    "devhost",
		IP:      "10.0.0.8",
		User:    "root",
		KeyPath: "~/.ssh/id_rsa",
		Remark:  "testing host",
		Group:   "testing",
		Port:    2222,
	}})
	for _, want := range []string{"Name", "Group", "Address", "Auth", "Remark", "devhost", "testing", "root@10.0.0.8:2222", "key:~/.ssh/id_rsa", "testing host"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table output %q does not contain %q", out, want)
		}
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

func newTestApp() *capp.App {
	app := capp.NewWith("sshc", "test", "simple ssh command runner")
	app.Add(NewAddCmd(), NewRunCmd(), NewUploadCmd(), NewDownloadCmd(), NewListCmd(), NewLogCmd(), NewLoginCmd())
	return app
}

func withTempConfig(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(core.SetUserHomeDirForTest(func() (string, error) { return home, nil }))

	path := filepath.Join(home, "hosts.json")
	t.Setenv(core.ConfigEnvKey, path)
	return path
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
