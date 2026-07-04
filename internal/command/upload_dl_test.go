package command

import (
	"strings"
	"testing"
	"time"

	"github.com/inhere/sshc/internal/core"
)

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

func TestSCPPassesJumpOption(t *testing.T) {
	saveJumpCommandHosts(t)

	var gotHost core.Host
	t.Cleanup(setUploadRemoteForTest(func(host core.Host, jobs []core.TransferJob, opts core.TransferOptions) (core.TransferResult, error) {
		gotHost = host
		return core.TransferResult{Bytes: 1, Files: 1, Elapsed: time.Second}, nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"scp", "-l", "app.jar", "-r", "/tmp/app.jar", "inner-db", "--jump", "bastion"}); err != nil {
		t.Fatalf("scp with jump: %v", err)
	}
	if gotHost.Jump != "bastion" {
		t.Fatalf("jump = %q, want bastion", gotHost.Jump)
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

func TestDownloadPassesJumpOption(t *testing.T) {
	saveJumpCommandHosts(t)

	var gotHost core.Host
	t.Cleanup(setDownloadRemoteForTest(func(host core.Host, remotePath, localPath string, opts core.TransferOptions) (core.TransferResult, error) {
		gotHost = host
		return core.TransferResult{Bytes: 1, Files: 1, Elapsed: time.Second}, nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"download", "-r", "/var/log/app.log", "-l", "tmp/logs", "inner-db", "--jump", "bastion"}); err != nil {
		t.Fatalf("download with jump: %v", err)
	}
	if gotHost.Jump != "bastion" {
		t.Fatalf("jump = %q, want bastion", gotHost.Jump)
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
