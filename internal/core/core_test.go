package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
		if got := JoinRemotePath(tt.base, tt.elem); got != tt.want {
			t.Fatalf("JoinRemotePath(%q, %q) = %q, want %q", tt.base, tt.elem, got, tt.want)
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
		if got := RemoteFilePath(tt.local, tt.remote); got != tt.want {
			t.Fatalf("RemoteFilePath(%q, %q) = %q, want %q", tt.local, tt.remote, got, tt.want)
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
		got, err := ParseTimeout(tt.value)
		if err != nil {
			t.Fatalf("ParseTimeout(%q): %v", tt.value, err)
		}
		if got != tt.want {
			t.Fatalf("ParseTimeout(%q) = %s, want %s", tt.value, got, tt.want)
		}
	}
}

func TestLoadRunEnvAndBuildRemoteCommand(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), "run.env")
	content := []byte("# comment\nexport FOO=file\nBAR=\"bar value\"\nEMPTY=\n")
	if err := os.WriteFile(envFile, content, 0600); err != nil {
		t.Fatal(err)
	}

	env, err := LoadRunEnv(envFile, []string{"FOO=inline", "QUOTE=a'b"})
	if err != nil {
		t.Fatal(err)
	}
	if env["FOO"] != "inline" || env["BAR"] != "bar value" || env["EMPTY"] != "" || env["QUOTE"] != "a'b" {
		t.Fatalf("env = %#v", env)
	}

	command, err := BuildRemoteCommand("echo ok", env)
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
		{name: "file explicit", fn: LocalFilePath, remote: "/tmp/a.txt", local: filepath.Join(dir, "renamed.txt"), want: filepath.Join(dir, "renamed.txt")},
		{name: "file existing dir", fn: LocalFilePath, remote: "/tmp/a.txt", local: existingDir, want: filepath.Join(existingDir, "a.txt")},
		{name: "file slash dir", fn: LocalFilePath, remote: "/tmp/a.txt", local: "downloads/", want: filepath.Join("downloads", "a.txt")},
		{name: "dir explicit", fn: LocalDirPath, remote: "/tmp/app", local: filepath.Join(dir, "app-copy"), want: filepath.Join(dir, "app-copy")},
		{name: "dir existing dir", fn: LocalDirPath, remote: "/tmp/app", local: existingDir, want: filepath.Join(existingDir, "app")},
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
		if got := RemoteRelPath(tt.root, tt.current); got != tt.want {
			t.Fatalf("RemoteRelPath(%q, %q) = %q, want %q", tt.root, tt.current, got, tt.want)
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
	t.Setenv(ConfigEnvKey, "")

	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(SetUserHomeDirForTest(func() (string, error) { return home, nil }))

	path, err := StorePath()
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
		if err := AppendRunLog(host, rec); err != nil {
			t.Fatalf("append log: %v", err)
		}
	}

	matched, err := ReadRunLogs("devhost", "beta", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 || !strings.Contains(matched[0], "echo beta") {
		t.Fatalf("matched logs = %#v", matched)
	}

	tailed, err := ReadRunLogs("devhost", "", 2)
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
	t.Cleanup(SetNowForTest(func() time.Time { return fixed }))

	host := Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
	if err := AppendRunLog(host, RunLogRecord{
		Target:    "devhost",
		Command:   "echo ok",
		Status:    "success",
		StartedAt: fixed,
	}); err != nil {
		t.Fatalf("append log: %v", err)
	}

	lines, err := ReadRunLogs("devhost", "", 10)
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

func withTempConfig(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(SetUserHomeDirForTest(func() (string, error) { return home, nil }))

	path := filepath.Join(home, "hosts.json")
	t.Setenv(ConfigEnvKey, path)
	return path
}
