package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/core"

	"github.com/gookit/goutil/cflag/capp"
)

func TestAddAndList(t *testing.T) {
	withTempConfig(t)

	app := newTestApp()
	err := app.RunWithArgs([]string{"add", "--ip", "10.0.0.8", "-u", "root", "-p", "secret", "--name", "devhost"})
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
	var gotLocal string
	var gotRemote string
	t.Cleanup(setUploadRemoteForTest(func(host core.Host, localPath, remotePath string) error {
		gotHost = host
		gotLocal = localPath
		gotRemote = remotePath
		return nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"scp", "-l", "local.txt", "-r", "/tmp/remote.txt", "devhost"}); err != nil {
		t.Fatalf("scp: %v", err)
	}
	if gotHost.IP != "10.0.0.8" {
		t.Fatalf("host ip = %q", gotHost.IP)
	}
	if gotLocal != "local.txt" || gotRemote != "/tmp/remote.txt" {
		t.Fatalf("paths = %q -> %q", gotLocal, gotRemote)
	}
}

func TestSCPRequiresSavedHost(t *testing.T) {
	withTempConfig(t)
	t.Cleanup(setUploadRemoteForTest(func(host core.Host, localPath, remotePath string) error {
		t.Fatal("upload should not be called")
		return nil
	}))

	app := newTestApp()
	err := app.RunWithArgs([]string{"scp", "-l", "local.txt", "-r", "/tmp/remote.txt", "missing"})
	if err == nil || !strings.Contains(err.Error(), `host "missing" not found`) {
		t.Fatalf("err = %v", err)
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
	t.Cleanup(setDownloadRemoteForTest(func(host core.Host, remotePath, localPath string) error {
		gotHost = host
		gotRemote = remotePath
		gotLocal = localPath
		return nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"download", "-r", "/tmp/remote.txt", "-l", "local.txt", "devhost"}); err != nil {
		t.Fatalf("download: %v", err)
	}
	if gotHost.IP != "10.0.0.8" {
		t.Fatalf("host ip = %q", gotHost.IP)
	}
	if gotRemote != "/tmp/remote.txt" || gotLocal != "local.txt" {
		t.Fatalf("paths = %q -> %q", gotRemote, gotLocal)
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

	t.Cleanup(setDownloadRemoteForTest(func(host core.Host, remotePath, localPath string) error { return nil }))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"dl", "-r", "/tmp/remote.txt", "-l", "local.txt", "devhost"}); err != nil {
		t.Fatalf("dl: %v", err)
	}
}

func TestDownloadRequiresSavedHost(t *testing.T) {
	withTempConfig(t)
	t.Cleanup(setDownloadRemoteForTest(func(host core.Host, remotePath, localPath string) error {
		t.Fatal("download should not be called")
		return nil
	}))

	app := newTestApp()
	err := app.RunWithArgs([]string{"download", "-r", "/tmp/remote.txt", "-l", "local.txt", "missing"})
	if err == nil || !strings.Contains(err.Error(), `host "missing" not found`) {
		t.Fatalf("err = %v", err)
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
	app.Add(NewAddCmd(), NewRunCmd(), NewUploadCmd(), NewDownloadCmd(), NewListCmd(), NewLogCmd())
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
