package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAddAndList(t *testing.T) {
	withTempConfig(t)

	app := newApp()
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
	store := &Store{Hosts: []Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     2222,
	}}}
	if err := saveStore(store); err != nil {
		t.Fatalf("save store: %v", err)
	}

	oldRun := runRemote
	t.Cleanup(func() { runRemote = oldRun })

	var gotHost Host
	var gotCommand string
	runRemote = func(host Host, command string, opts RunOptions) ([]byte, error) {
		gotHost = host
		gotCommand = command
		return []byte("ok\n"), nil
	}

	app := newApp()

	if err := app.RunWithArgs([]string{"run", "devhost", "--", "echo", "hello"}); err != nil {
		t.Fatalf("run host: %v", err)
	}
	if gotHost.IP != "10.0.0.8" {
		t.Fatalf("host ip = %q", gotHost.IP)
	}
	if gotCommand != "echo hello" {
		t.Fatalf("command = %q", gotCommand)
	}

	lines, err := readRunLogs("devhost", "", 10)
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

func TestRunPassesTimeoutAndEnvOptions(t *testing.T) {
	withTempConfig(t)
	envFile := filepath.Join(t.TempDir(), "run.env")
	if err := os.WriteFile(envFile, []byte("FOO=file\nBAR=bar\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &Store{Hosts: []Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     2222,
	}}}
	if err := saveStore(store); err != nil {
		t.Fatalf("save store: %v", err)
	}

	oldRun := runRemote
	t.Cleanup(func() { runRemote = oldRun })

	var gotOpts RunOptions
	runRemote = func(host Host, command string, opts RunOptions) ([]byte, error) {
		gotOpts = opts
		return []byte("ok\n"), nil
	}

	app := newApp()
	err := app.RunWithArgs([]string{
		"run",
		"--timeout", "3s",
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
	if gotOpts.Env["FOO"] != "inline" || gotOpts.Env["BAR"] != "bar" || gotOpts.Env["BAZ"] != "baz" {
		t.Fatalf("env = %#v", gotOpts.Env)
	}
}

func TestSCPUsesSavedHost(t *testing.T) {
	withTempConfig(t)
	store := &Store{Hosts: []Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     2222,
	}}}
	if err := saveStore(store); err != nil {
		t.Fatalf("save store: %v", err)
	}

	oldUpload := scpUpload
	t.Cleanup(func() { scpUpload = oldUpload })

	var gotHost Host
	var gotLocal string
	var gotRemote string
	scpUpload = func(host Host, localPath, remotePath string) error {
		gotHost = host
		gotLocal = localPath
		gotRemote = remotePath
		return nil
	}

	app := newApp()
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
	oldUpload := scpUpload
	t.Cleanup(func() { scpUpload = oldUpload })
	scpUpload = func(host Host, localPath, remotePath string) error {
		t.Fatal("upload should not be called")
		return nil
	}

	app := newApp()
	err := app.RunWithArgs([]string{"scp", "-l", "local.txt", "-r", "/tmp/remote.txt", "missing"})
	if err == nil || !strings.Contains(err.Error(), `host "missing" not found`) {
		t.Fatalf("err = %v", err)
	}
}

func TestDownloadUsesSavedHost(t *testing.T) {
	withTempConfig(t)
	store := &Store{Hosts: []Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     2222,
	}}}
	if err := saveStore(store); err != nil {
		t.Fatalf("save store: %v", err)
	}

	oldDownload := downloadRemote
	t.Cleanup(func() { downloadRemote = oldDownload })

	var gotHost Host
	var gotRemote string
	var gotLocal string
	downloadRemote = func(host Host, remotePath, localPath string) error {
		gotHost = host
		gotRemote = remotePath
		gotLocal = localPath
		return nil
	}

	app := newApp()
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
	store := &Store{Hosts: []Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     2222,
	}}}
	if err := saveStore(store); err != nil {
		t.Fatalf("save store: %v", err)
	}

	oldDownload := downloadRemote
	t.Cleanup(func() { downloadRemote = oldDownload })
	downloadRemote = func(host Host, remotePath, localPath string) error { return nil }

	app := newApp()
	if err := app.RunWithArgs([]string{"dl", "-r", "/tmp/remote.txt", "-l", "local.txt", "devhost"}); err != nil {
		t.Fatalf("dl: %v", err)
	}
}

func TestDownloadRequiresSavedHost(t *testing.T) {
	withTempConfig(t)
	oldDownload := downloadRemote
	t.Cleanup(func() { downloadRemote = oldDownload })
	downloadRemote = func(host Host, remotePath, localPath string) error {
		t.Fatal("download should not be called")
		return nil
	}

	app := newApp()
	err := app.RunWithArgs([]string{"download", "-r", "/tmp/remote.txt", "-l", "local.txt", "missing"})
	if err == nil || !strings.Contains(err.Error(), `host "missing" not found`) {
		t.Fatalf("err = %v", err)
	}
}

func TestJoinRemotePath(t *testing.T) {
	tests := []struct {
		base string
		elem string
		want string
	}{
		{base: "/opt/app", elem: "a.txt", want: "/opt/app/a.txt"},
		{base: "/opt/app/", elem: "dir/a.txt", want: "/opt/app/dir/a.txt"},
		{base: ".", elem: "a.txt", want: "a.txt"},
	}
	for _, tt := range tests {
		if got := joinRemotePath(tt.base, tt.elem); got != tt.want {
			t.Fatalf("joinRemotePath(%q, %q) = %q, want %q", tt.base, tt.elem, got, tt.want)
		}
	}
}

func TestRemoteFilePath(t *testing.T) {
	tests := []struct {
		local  string
		remote string
		want   string
	}{
		{local: "local.txt", remote: "/tmp/remote.txt", want: "/tmp/remote.txt"},
		{local: "local.txt", remote: "/tmp/", want: "/tmp/local.txt"},
	}
	for _, tt := range tests {
		if got := remoteFilePath(tt.local, tt.remote); got != tt.want {
			t.Fatalf("remoteFilePath(%q, %q) = %q, want %q", tt.local, tt.remote, got, tt.want)
		}
	}
}

func TestParseTimeout(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
	}{
		{value: "", want: 0},
		{value: "5", want: 5 * time.Second},
		{value: "1500ms", want: 1500 * time.Millisecond},
		{value: "2m", want: 2 * time.Minute},
	}
	for _, tt := range tests {
		got, err := parseTimeout(tt.value)
		if err != nil {
			t.Fatalf("parseTimeout(%q): %v", tt.value, err)
		}
		if got != tt.want {
			t.Fatalf("parseTimeout(%q) = %s, want %s", tt.value, got, tt.want)
		}
	}
}

func TestLoadRunEnvAndBuildRemoteCommand(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), "run.env")
	content := []byte("# comment\nexport FOO=file\nBAR=\"bar value\"\nEMPTY=\n")
	if err := os.WriteFile(envFile, content, 0600); err != nil {
		t.Fatal(err)
	}

	env, err := loadRunEnv(envFile, []string{"FOO=inline", "QUOTE=a'b"})
	if err != nil {
		t.Fatal(err)
	}
	if env["FOO"] != "inline" || env["BAR"] != "bar value" || env["EMPTY"] != "" || env["QUOTE"] != "a'b" {
		t.Fatalf("env = %#v", env)
	}

	command, err := buildRemoteCommand("echo ok", env)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"BAR='bar value'", "EMPTY=''", "FOO='inline'", `QUOTE='a'\''b'`, "echo ok"} {
		if !strings.Contains(command, want) {
			t.Fatalf("command %q does not contain %q", command, want)
		}
	}
}

func TestLocalDownloadPaths(t *testing.T) {
	dir := t.TempDir()
	existingDir := filepath.Join(dir, "existing")
	if err := os.Mkdir(existingDir, 0700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		fn     func(string, string) string
		remote string
		local  string
		want   string
	}{
		{name: "file explicit", fn: localFilePath, remote: "/tmp/a.txt", local: filepath.Join(dir, "renamed.txt"), want: filepath.Join(dir, "renamed.txt")},
		{name: "file existing dir", fn: localFilePath, remote: "/tmp/a.txt", local: existingDir, want: filepath.Join(existingDir, "a.txt")},
		{name: "file slash dir", fn: localFilePath, remote: "/tmp/a.txt", local: "downloads/", want: filepath.Join("downloads", "a.txt")},
		{name: "dir explicit", fn: localDirPath, remote: "/tmp/app", local: filepath.Join(dir, "app-copy"), want: filepath.Join(dir, "app-copy")},
		{name: "dir existing dir", fn: localDirPath, remote: "/tmp/app", local: existingDir, want: filepath.Join(existingDir, "app")},
	}
	for _, tt := range tests {
		if got := tt.fn(tt.remote, tt.local); got != tt.want {
			t.Fatalf("%s = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestRemoteRelPath(t *testing.T) {
	tests := []struct {
		root    string
		current string
		want    string
	}{
		{root: "/tmp/app", current: "/tmp/app", want: ""},
		{root: "/tmp/app", current: "/tmp/app/conf", want: "conf"},
		{root: "/tmp/app", current: "/tmp/app/conf/app.yaml", want: "conf/app.yaml"},
	}
	for _, tt := range tests {
		if got := remoteRelPath(tt.root, tt.current); got != tt.want {
			t.Fatalf("remoteRelPath(%q, %q) = %q, want %q", tt.root, tt.current, got, tt.want)
		}
	}
}

func TestStoreUpsertReplacesByNameOrIP(t *testing.T) {
	store := &Store{}
	if err := store.Upsert(Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "one", Port: 22}); err != nil {
		t.Fatal(err)
	}
	if err := store.Upsert(Host{Name: "devhost", IP: "10.0.0.9", User: "ops", Password: "two", Port: 22}); err != nil {
		t.Fatal(err)
	}
	if len(store.Hosts) != 1 {
		t.Fatalf("hosts len = %d, want 1", len(store.Hosts))
	}
	if store.Hosts[0].IP != "10.0.0.9" || store.Hosts[0].User != "ops" {
		t.Fatalf("unexpected replacement: %+v", store.Hosts[0])
	}
}

func TestStorePathDefaultsToDotConfig(t *testing.T) {
	t.Setenv(configEnvKey, "")

	home := filepath.Join(t.TempDir(), "home")
	oldUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = oldUserHomeDir })

	path, err := storePath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "sshc", "hosts.json")
	if path != want {
		t.Fatalf("store path = %q, want %q", path, want)
	}
}

func TestReadRunLogsMatchesAndTails(t *testing.T) {
	withTempConfig(t)
	host := Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}

	records := []RunLogRecord{
		{Target: "devhost", Command: "echo alpha", Status: "success"},
		{Target: "devhost", Command: "echo beta", Status: "success"},
		{Target: "devhost", Command: "echo gamma", Status: "success"},
	}
	for _, rec := range records {
		if err := appendRunLog(host, rec); err != nil {
			t.Fatalf("append log: %v", err)
		}
	}

	matched, err := readRunLogs("devhost", "beta", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 || !strings.Contains(matched[0], "echo beta") {
		t.Fatalf("matched logs = %#v", matched)
	}

	tailed, err := readRunLogs("devhost", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(tailed) != 2 || !strings.Contains(tailed[0], "echo beta") || !strings.Contains(tailed[1], "echo gamma") {
		t.Fatalf("tailed logs = %#v", tailed)
	}
}

func TestRunLogTimeFormatUsesMillisecondsWithoutZone(t *testing.T) {
	withTempConfig(t)
	loc := time.FixedZone("CST", 8*60*60)
	fixed := time.Date(2026, 7, 3, 17, 16, 14, 350724100, loc)
	oldNow := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = oldNow })

	host := Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
	if err := appendRunLog(host, RunLogRecord{
		Target:    "devhost",
		Command:   "echo ok",
		Status:    "success",
		StartedAt: fixed,
	}); err != nil {
		t.Fatalf("append log: %v", err)
	}

	lines, err := readRunLogs("devhost", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("log lines len = %d, want 1", len(lines))
	}
	line := lines[0]
	for _, want := range []string{`"time":"2026-07-03T17:16:14.350"`, `"started_at":"2026-07-03T17:16:14.350"`} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q does not contain %q", line, want)
		}
	}
	if strings.Contains(line, "+08") || strings.Contains(line, "3507241") {
		t.Fatalf("log line has unwanted zone or sub-millisecond precision: %q", line)
	}
}

func TestResolveLogTargetUsesSavedHost(t *testing.T) {
	withTempConfig(t)
	store := &Store{Hosts: []Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     22,
	}}}
	if err := saveStore(store); err != nil {
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

func withTempConfig(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	oldUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = oldUserHomeDir })

	path := filepath.Join(home, "hosts.json")
	t.Setenv(configEnvKey, path)
	return path
}

func readTestStore(t *testing.T) Store {
	t.Helper()
	path, err := storePath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		t.Fatal(err)
	}
	return store
}
