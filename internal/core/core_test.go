package core

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
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

func TestBuildRemoteCommandWithCWD(t *testing.T) {
	command, err := BuildRemoteCommandWithCWD("python -m app", map[string]string{"APP_ENV": "prod"}, "/opt/my app")
	if err != nil {
		t.Fatal(err)
	}
	want := "cd '/opt/my app' && APP_ENV='prod' python -m app"
	if command != want {
		t.Fatalf("command = %q, want %q", command, want)
	}
}

func TestScriptExecuteCommandQuotesPath(t *testing.T) {
	command := scriptExecuteCommand("/tmp/sshc run/a'b.sh")
	want := `bash '/tmp/sshc run/a'\''b.sh'`
	if command != want {
		t.Fatalf("command = %q, want %q", command, want)
	}
}

func TestRemoteScriptMode(t *testing.T) {
	if got := remoteScriptMode(RunOptions{}); got != "700" {
		t.Fatalf("default script mode = %q, want 700", got)
	}
	if got := remoteScriptMode(RunOptions{SudoUser: "app"}); got != "644" {
		t.Fatalf("sudo-user script mode = %q, want 644", got)
	}
}

func TestRemoteTimeoutCommand(t *testing.T) {
	command := remoteTimeoutCommand("cd '/opt/app' && python -m app", RunOptions{
		Timeout:   10 * time.Second,
		KillAfter: 2 * time.Second,
	})
	for _, want := range []string{
		"command -v timeout",
		"timeout --kill-after=2s 10s bash -lc",
		`'cd '\''/opt/app'\'' && python -m app'`,
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command %q does not contain %q", command, want)
		}
	}
}

func TestRemoteSudoCommand(t *testing.T) {
	command := remoteSudoCommand("cd /opt/app && whoami", RunOptions{Sudo: true})
	want := `sudo bash -lc 'cd /opt/app && whoami'`
	if command != want {
		t.Fatalf("command = %q, want %q", command, want)
	}
}

func TestRemoteSudoUserCommand(t *testing.T) {
	command := remoteSudoCommand("whoami", RunOptions{SudoUser: "ylpy"})
	want := `sudo -u 'ylpy' bash -lc 'whoami'`
	if command != want {
		t.Fatalf("command = %q, want %q", command, want)
	}
}

func TestValidateSudoUser(t *testing.T) {
	for _, user := range []string{"ylpy", "app-user", "user_1", "svc$"} {
		if err := ValidateSudoUser(user); err != nil {
			t.Fatalf("ValidateSudoUser(%q): %v", user, err)
		}
	}
	for _, user := range []string{"", "bad user", "bad;user"} {
		if err := ValidateSudoUser(user); err == nil {
			t.Fatalf("ValidateSudoUser(%q) nil error", user)
		}
	}
}

func TestRemoteClientTimeoutAddsKillAfterAndBuffer(t *testing.T) {
	got := remoteClientTimeout(RunOptions{Timeout: 10 * time.Second, KillAfter: 2 * time.Second})
	want := 17 * time.Second
	if got != want {
		t.Fatalf("client timeout = %s, want %s", got, want)
	}
}

func TestEffectiveKillAfterDefault(t *testing.T) {
	if got := effectiveKillAfter(0); got != defaultRemoteKillAfter {
		t.Fatalf("kill after = %s, want %s", got, defaultRemoteKillAfter)
	}
}

func TestNewRemoteScriptPath(t *testing.T) {
	path := NewRemoteScriptPath(time.Unix(123, 456))
	if !strings.HasPrefix(path, "/tmp/sshc-run-") || !strings.HasSuffix(path, ".sh") {
		t.Fatalf("path = %q", path)
	}
}

func TestNewRemoteScriptPathInDir(t *testing.T) {
	path := NewRemoteScriptPathInDir(time.Unix(123, 456), "/opt/app/tmp")
	if !strings.HasPrefix(path, "/opt/app/tmp/sshc-run-") || !strings.HasSuffix(path, ".sh") {
		t.Fatalf("path = %q", path)
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

func TestFileSHA256(t *testing.T) {
	file := filepath.Join(t.TempDir(), "data.txt")
	if err := os.WriteFile(file, []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := fileSHA256(file)
	if err != nil {
		t.Fatal(err)
	}
	want := "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"
	if got != want {
		t.Fatalf("sha256 = %q, want %q", got, want)
	}
}

func TestParseSHA256SumOutput(t *testing.T) {
	got, err := parseSHA256SumOutput("ABCDEFabcdefABCDEFabcdefABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD  /tmp/a.txt\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	if got != want {
		t.Fatalf("sha256 = %q, want %q", got, want)
	}
	if _, err := parseSHA256SumOutput("bad /tmp/a.txt"); err == nil {
		t.Fatal("expected invalid sha256sum output error")
	}
}

func TestVerifySHA256Mismatch(t *testing.T) {
	err := verifySHA256("aaa", "bbb")
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateRemoteRemoveDirPath(t *testing.T) {
	for _, remotePath := range []string{"", ".", "/"} {
		if err := validateRemoteRemoveDirPath(remotePath); err == nil {
			t.Fatalf("validateRemoteRemoveDirPath(%q) nil error", remotePath)
		}
	}
	if err := validateRemoteRemoveDirPath("/opt/app/dist"); err != nil {
		t.Fatalf("validateRemoteRemoveDirPath valid path: %v", err)
	}
}

func TestExpandLocalGlob(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.jar")
	fileB := filepath.Join(dir, "b.jar")
	if err := os.WriteFile(fileB, []byte("b"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileA, []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := expandLocalGlob(filepath.Join(dir, "*.jar"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{fileA, fileB}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("matches = %#v, want %#v", got, want)
	}
}

func TestExpandLocalGlobRejectsEmptyAndDirectories(t *testing.T) {
	dir := t.TempDir()
	if _, err := expandLocalGlob(filepath.Join(dir, "*.jar")); err == nil {
		t.Fatal("expected empty glob error")
	}
	subdir := filepath.Join(dir, "sub.jar")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := expandLocalGlob(filepath.Join(dir, "*.jar")); err == nil || !strings.Contains(err.Error(), "matched directory") {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadRemoteRejectsRemoveDirForFileBeforeConnect(t *testing.T) {
	file := filepath.Join(t.TempDir(), "data.txt")
	if err := os.WriteFile(file, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := UploadRemote(Host{IP: "127.0.0.1", User: "root", Port: 1}, file, "/tmp/data.txt", TransferOptions{RemoveDir: true})
	if err == nil || !strings.Contains(err.Error(), "only supported for directory uploads") {
		t.Fatalf("err = %v", err)
	}
}

func TestExpandUploadJobsRepeatedLocalUsesRemoteDirectory(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.jar")
	fileB := filepath.Join(dir, "b.jar")
	for _, file := range []string{fileA, fileB} {
		if err := os.WriteFile(file, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	jobs, err := expandUploadJobs([]TransferJob{
		{LocalPath: fileA, RemotePath: "/opt/app/lib", RemoteDir: true},
		{LocalPath: fileB, RemotePath: "/opt/app/lib", RemoteDir: true},
	}, TransferOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs len = %d, want 2", len(jobs))
	}
	if jobs[0].RemotePath != "/opt/app/lib/a.jar" || jobs[1].RemotePath != "/opt/app/lib/b.jar" {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestExpandUploadJobsKeepsMappedRemotePath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "app.yml")
	if err := os.WriteFile(file, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	jobs, err := expandUploadJobs([]TransferJob{{LocalPath: file, RemotePath: "/etc/app/app.yml"}}, TransferOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].RemotePath != "/etc/app/app.yml" {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestExpandUploadJobsExpandsGlobToRemoteDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.jar", "b.jar"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	jobs, err := expandUploadJobs([]TransferJob{{LocalPath: filepath.Join(dir, "*.jar"), RemotePath: "/opt/app/lib"}}, TransferOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs len = %d, want 2", len(jobs))
	}
	if jobs[0].RemotePath != "/opt/app/lib/a.jar" || jobs[1].RemotePath != "/opt/app/lib/b.jar" {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestExpandUploadJobsRejectsSHA256Directory(t *testing.T) {
	_, err := expandUploadJobs([]TransferJob{{LocalPath: t.TempDir(), RemotePath: "/opt/app/dist"}}, TransferOptions{SHA256: true})
	if err == nil || !strings.Contains(err.Error(), "--sha256 is only supported for file transfers") {
		t.Fatalf("err = %v", err)
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

func TestValidateHostAllowsKeyPathWithoutPassword(t *testing.T) {
	err := validateHost(Host{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22})
	if err != nil {
		t.Fatalf("validateHost with key path: %v", err)
	}
	err = validateHost(Host{Name: "devhost", IP: "10.0.0.8", User: "root", Port: 22})
	if err == nil || !strings.Contains(err.Error(), "password or key_path") {
		t.Fatalf("err = %v", err)
	}
}

func TestSaveStoreEncryptsPasswordAndLoadDecrypts(t *testing.T) {
	path := withTempConfig(t)
	storeToSave := &Store{Hosts: []Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     22,
	}}}
	if err := SaveStore(storeToSave); err != nil {
		t.Fatal(err)
	}
	if storeToSave.Hosts[0].Password != "secret" {
		t.Fatalf("SaveStore mutated source password: %+v", storeToSave.Hosts[0])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, `"password":"secret"`) || strings.Contains(content, `"password": "secret"`) {
		t.Fatalf("stored config contains plaintext password: %s", content)
	}
	if !strings.Contains(content, `"password_enc": "v1:`) {
		t.Fatalf("stored config does not contain encrypted password: %s", content)
	}

	store, err := LoadStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Hosts) != 1 || store.Hosts[0].Password != "secret" {
		t.Fatalf("loaded store = %+v", store.Hosts)
	}
	keyPath, err := PasswordKeyPath()
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(keyPath); err != nil || info.IsDir() {
		t.Fatalf("password key file not created: info=%v err=%v", info, err)
	}
}

func TestLoadStoreSupportsLegacyPlaintextPassword(t *testing.T) {
	path := withTempConfig(t)
	data := []byte(`{"hosts":[{"name":"devhost","ip":"10.0.0.8","user":"root","password":"secret","port":22}]}` + "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	store, err := LoadStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Hosts) != 1 || store.Hosts[0].Password != "secret" {
		t.Fatalf("loaded store = %+v", store.Hosts)
	}
}

func TestLoadConfigLegacyStoreShape(t *testing.T) {
	path := withTempConfig(t)
	data := []byte(`{"logs_path":"runtime/logs","hosts":[{"name":"devhost","ip":"10.0.0.8","user":"root","password":"secret","port":22}]}` + "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != ConfigVersion || config.LogsPath != "runtime/logs" {
		t.Fatalf("config = %+v", config)
	}
	if len(config.AuthProfiles) != 0 {
		t.Fatalf("auth profiles = %+v", config.AuthProfiles)
	}
	if len(config.Hosts) != 1 || config.Hosts[0].Password != "secret" {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestLoadConfigVersionedShape(t *testing.T) {
	path := withTempConfig(t)
	data := []byte(`{
  "version": 1,
  "logs_path": "logs",
  "defaults": {"user":"root","port":22},
  "auth_profiles": [{"name":"dev-root","user":"root","password":"secret"}],
  "hosts": [{"name":"devhost","ip":"10.0.0.8","auth_ref":"dev-root","port":22}]
}` + "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != ConfigVersion || config.Defaults.User != "root" {
		t.Fatalf("config = %+v", config)
	}
	if len(config.AuthProfiles) != 1 || config.AuthProfiles[0].Name != "dev-root" || config.AuthProfiles[0].Password != "secret" {
		t.Fatalf("auth profiles = %+v", config.AuthProfiles)
	}
	if len(config.Hosts) != 1 || config.Hosts[0].AuthRef != "dev-root" {
		t.Fatalf("hosts = %+v", config.Hosts)
	}
}

func TestLoadConfigFromLegacyHostsFile(t *testing.T) {
	t.Setenv(ConfigEnvKey, "")
	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(SetUserHomeDirForTest(func() (string, error) { return home, nil }))

	legacyPath := filepath.Join(home, ".config", "sshc", LegacyConfigFileName)
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"hosts":[{"name":"devhost","ip":"10.0.0.8","user":"root","password":"secret","port":22}]}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != ConfigVersion || len(config.Hosts) != 1 || config.Hosts[0].Name != "devhost" {
		t.Fatalf("config = %+v", config)
	}
}

func TestLoadConfigWithEnvPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom-config.json")
	t.Setenv(ConfigEnvKey, path)
	if err := os.WriteFile(path, []byte(`{"logs_path":"custom/logs","hosts":[]}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.LogsPath != "custom/logs" {
		t.Fatalf("logs_path = %q", config.LogsPath)
	}
}

func TestSaveConfigWritesVersionOne(t *testing.T) {
	path := withTempConfig(t)
	config := &Config{
		LogsPath:     "runtime/logs",
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}},
		Hosts:        []Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root"}},
	}
	if err := SaveConfig(config); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Version != ConfigVersion {
		t.Fatalf("version = %d, want %d; json=%s", saved.Version, ConfigVersion, string(data))
	}
	if config.Version != 0 {
		t.Fatalf("SaveConfig mutated source version: %+v", config)
	}
}

func TestLoadStoreStillDecryptsHostPasswordEnc(t *testing.T) {
	path := withTempConfig(t)
	encrypted, err := EncryptPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"version":1,"hosts":[{"name":"devhost","ip":"10.0.0.8","user":"root","password_enc":"` + encrypted + `","port":22}]}` + "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	store, err := LoadStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Hosts) != 1 || store.Hosts[0].Password != "secret" {
		t.Fatalf("store = %+v", store)
	}
}

func TestLoadConfigSettingsReadsLogsPath(t *testing.T) {
	path := withTempConfig(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1,"logs_path":"runtime/logs","hosts":[]}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	settings, err := LoadConfigSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.LogsPath != "runtime/logs" {
		t.Fatalf("logs_path = %q", settings.LogsPath)
	}
}

func TestEncryptPasswordRoundTrip(t *testing.T) {
	withTempConfig(t)
	encrypted, err := EncryptPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "secret" || !strings.HasPrefix(encrypted, "v1:") {
		t.Fatalf("encrypted password = %q", encrypted)
	}
	plain, err := DecryptPassword(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "secret" {
		t.Fatalf("plain = %q, want secret", plain)
	}
}

func TestDecryptPasswordMissingKeyDoesNotCreateNewKey(t *testing.T) {
	withTempConfig(t)
	_, err := DecryptPassword("v1:AAAA")
	if err == nil || !strings.Contains(err.Error(), "password key file not found") {
		t.Fatalf("err = %v", err)
	}
	keyPath, err := PasswordKeyPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("key file should not be created, err=%v", err)
	}
}

func TestExpandUserPath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(SetUserHomeDirForTest(func() (string, error) { return home, nil }))

	got := expandUserPath("~/.ssh/id_rsa")
	want := filepath.Join(home, ".ssh", "id_rsa")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestHostAuthUsesKeyAndPassword(t *testing.T) {
	keyPath := writeTestPrivateKey(t)
	auth, err := hostAuth(Host{KeyPath: keyPath, Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if len(auth) != 3 {
		t.Fatalf("auth methods = %d, want 3", len(auth))
	}
}

func writeTestPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	path := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStoreResolveHostUsesExactMatchFirst(t *testing.T) {
	store := Store{Hosts: []Host{
		{Name: "dev", IP: "10.0.0.8", User: "root", Password: "one", Port: 22},
		{Name: "devhost", IP: "10.0.0.9", User: "root", Password: "two", Port: 22},
	}}

	host, ok, err := store.ResolveHost("dev")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || host.Name != "dev" {
		t.Fatalf("host = %+v, ok = %v", host, ok)
	}
}

func TestStoreResolveHostMatchesUniqueParts(t *testing.T) {
	store := Store{Hosts: []Host{
		{Name: "testing-web", IP: "10.0.0.8", User: "root", Password: "one", Port: 22},
		{Name: "testing-db", IP: "10.0.0.9", User: "root", Password: "two", Port: 22},
	}}

	host, ok, err := store.ResolveHost("test web")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || host.Name != "testing-web" {
		t.Fatalf("host = %+v, ok = %v", host, ok)
	}
}

func TestStoreResolveHostMatchesRemarkAndGroup(t *testing.T) {
	store := Store{Hosts: []Host{
		{Name: "web-a", IP: "10.0.0.8", User: "root", Password: "one", Remark: "gpu runner", Group: "testing", Port: 22},
		{Name: "web-b", IP: "10.0.0.9", User: "root", Password: "two", Remark: "api server", Group: "prod", Port: 22},
	}}

	host, ok, err := store.ResolveHost("testing gpu")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || host.Name != "web-a" {
		t.Fatalf("host = %+v, ok = %v", host, ok)
	}
}

func TestStoreResolveHostRejectsMultiplePartialMatches(t *testing.T) {
	store := Store{Hosts: []Host{
		{Name: "testing-web", IP: "10.0.0.8", User: "root", Password: "one", Port: 22},
		{Name: "testing-db", IP: "10.0.0.9", User: "root", Password: "two", Port: 22},
	}}

	_, ok, err := store.ResolveHost("testing")
	if err == nil {
		t.Fatal("expected multiple match error")
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
	for _, want := range []string{"testing-web", "testing-db"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err %q does not contain %q", err.Error(), want)
		}
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
	want := filepath.Join(home, ".config", "sshc", ConfigFileName)
	if path != want {
		t.Fatalf("store path = %q, want %q", path, want)
	}
}

func TestLoadStoreSupportsLegacyHostsJSON(t *testing.T) {
	t.Setenv(ConfigEnvKey, "")
	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(SetUserHomeDirForTest(func() (string, error) { return home, nil }))

	legacyPath := filepath.Join(home, ".config", "sshc", LegacyConfigFileName)
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"hosts":[{"name":"devhost","ip":"10.0.0.8","user":"root","password":"secret","port":22}]}` + "\n")
	if err := os.WriteFile(legacyPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	store, err := LoadStore()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Hosts) != 1 || store.Hosts[0].Name != "devhost" {
		t.Fatalf("loaded store = %+v", store)
	}
}

func TestRunLogDirUsesConfiguredLogsPath(t *testing.T) {
	path := withTempConfig(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"logs_path":"runtime/logs"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	dir, err := runLogDir()
	if err != nil {
		t.Fatal(err)
	}
	home, err := userHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "sshc", "runtime", "logs")
	if dir != want {
		t.Fatalf("log dir = %q, want %q", dir, want)
	}
}

func TestRunLogDirExpandsConfiguredLogsPath(t *testing.T) {
	path := withTempConfig(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"logs_path":"~/sshc-logs"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	dir, err := runLogDir()
	if err != nil {
		t.Fatal(err)
	}
	home, err := userHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "sshc-logs")
	if dir != want {
		t.Fatalf("log dir = %q, want %q", dir, want)
	}
}

func TestParseSSHConfig(t *testing.T) {
	config := `
Host *
  User ignored

Host devhost
  HostName 10.0.0.8
  User root
  Port 2222
  IdentityFile ~/.ssh/id_rsa

Host *.internal
  HostName ignored
  User ignored
  IdentityFile ~/.ssh/ignored
`
	hosts, err := ParseSSHConfig(strings.NewReader(config))
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Fatalf("hosts len = %d, want 1: %+v", len(hosts), hosts)
	}
	host := hosts[0]
	if host.Name != "devhost" || host.IP != "10.0.0.8" || host.User != "root" || host.Port != 2222 || host.KeyPath != "~/.ssh/id_rsa" {
		t.Fatalf("host = %+v", host)
	}
	if host.Group != "ssh-config" {
		t.Fatalf("group = %q", host.Group)
	}
}

func TestLoadStoreWithSSHConfigKeepsExplicitHost(t *testing.T) {
	withTempConfig(t)
	home, err := userHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	config := "Host devhost\n  HostName 10.0.0.9\n  User ops\n  IdentityFile ~/.ssh/id_rsa\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveStore(&Store{Hosts: []Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     22,
	}}}); err != nil {
		t.Fatal(err)
	}

	store, err := LoadStoreWithSSHConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Hosts) != 1 {
		t.Fatalf("hosts len = %d, want 1", len(store.Hosts))
	}
	if store.Hosts[0].IP != "10.0.0.8" {
		t.Fatalf("explicit host was not preserved: %+v", store.Hosts[0])
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

func TestLoginTermName(t *testing.T) {
	t.Setenv("TERM", "screen-256color")
	if got := loginTermName(""); got != "screen-256color" {
		t.Fatalf("term = %q, want screen-256color", got)
	}
	if got := loginTermName(" vt100 "); got != "vt100" {
		t.Fatalf("explicit term = %q, want vt100", got)
	}
	t.Setenv("TERM", "")
	if got := loginTermName(""); got != defaultPTYTerm {
		t.Fatalf("default term = %q, want %s", got, defaultPTYTerm)
	}
}

func withTempConfig(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(SetUserHomeDirForTest(func() (string, error) { return home, nil }))

	path := filepath.Join(home, ConfigFileName)
	t.Setenv(ConfigEnvKey, path)
	return path
}
