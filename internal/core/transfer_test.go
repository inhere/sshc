package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestUploadRemoteRejectsCommandProxyHost(t *testing.T) {
	file := filepath.Join(t.TempDir(), "data.txt")
	if err := os.WriteFile(file, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := UploadRemote(Host{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host"}, file, "/tmp/data.txt", TransferOptions{})
	if err == nil || !strings.Contains(err.Error(), "uses command_proxy backend") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchRemoteRejectsCommandProxyHost(t *testing.T) {
	_, err := FetchRemote(Host{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host"}, "/tmp/data.txt", "data.txt", TransferOptions{})
	if err == nil || !strings.Contains(err.Error(), "uses command_proxy backend") {
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

func TestEstimateUploadBytes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dist")
	if err := os.MkdirAll(filepath.Join(dir, "conf"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(dir, "app.jar"):      "ab",
		filepath.Join(dir, "conf/app.yml"): "cde",
		filepath.Join(root, "a.bin"):       "f",
		filepath.Join(root, "b.bin"):       "g",
	}
	for file, data := range files {
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}

	total, err := EstimateUploadBytes([]TransferJob{
		{LocalPath: dir, RemotePath: "/opt/app/dist"},
		{LocalPath: filepath.Join(root, "*.bin"), RemotePath: "/opt/app/bin"},
	}, TransferOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 7 {
		t.Fatalf("total = %d, want 7", total)
	}
}

func TestProgressWriterReportsBytes(t *testing.T) {
	var buf bytes.Buffer
	var got int64
	writer := progressWriter{
		writer: &buf,
		progress: func(n int64) {
			got += n
		},
	}

	n, err := writer.Write([]byte("abc"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || got != 3 || buf.String() != "abc" {
		t.Fatalf("n=%d got=%d buf=%q", n, got, buf.String())
	}
}
