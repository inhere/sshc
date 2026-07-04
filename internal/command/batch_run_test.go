package command

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/inhere/sshc/internal/core"
)

func TestBatchRunUsesHosts(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveStore(&core.Store{Hosts: []core.Host{
		{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22},
		{Name: "web-2", IP: "10.0.0.9", User: "root", Password: "secret", Port: 22},
	}}); err != nil {
		t.Fatalf("save store: %v", err)
	}

	var gotHosts []string
	var gotCommands []string
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotHosts = append(gotHosts, host.Name)
		gotCommands = append(gotCommands, command)
		return []byte("ok\n"), nil
	}))
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"batch-run", "--hosts", "devhost,web-2", "--parallel", "1", "--", "hostname"}); err != nil {
		t.Fatalf("batch-run: %v", err)
	}
	if strings.Join(gotHosts, ",") != "devhost,web-2" {
		t.Fatalf("hosts = %#v", gotHosts)
	}
	if len(gotCommands) != 2 || gotCommands[0] != "hostname" || gotCommands[1] != "hostname" {
		t.Fatalf("commands = %#v", gotCommands)
	}
	output := out.String()
	for _, want := range []string{"==> devhost", "==> web-2", "Summary: total=2 success=2 failed=0 skipped=0"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
}

func TestBatchRunAlias(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveStore(&core.Store{Hosts: []core.Host{{
		Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22,
	}}}); err != nil {
		t.Fatalf("save store: %v", err)
	}
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"brun", "--hosts", "devhost", "--", "uptime"}); err != nil {
		t.Fatalf("brun: %v", err)
	}
}

func TestBatchRunPassesRunOptions(t *testing.T) {
	withTempConfig(t)
	script := filepath.Join(t.TempDir(), "deploy.sh")
	if err := os.WriteFile(script, []byte("echo ok\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := core.SaveStore(&core.Store{Hosts: []core.Host{{
		Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22,
	}}}); err != nil {
		t.Fatalf("save store: %v", err)
	}

	var gotOpts core.RunOptions
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotOpts = opts
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"batch-run", "--hosts", "devhost", "--script", script, "--cwd", "/opt/app", "-e", "APP_ENV=prod", "--timeout", "30s"}); err != nil {
		t.Fatalf("batch-run: %v", err)
	}
	if gotOpts.ScriptPath != script || gotOpts.CWD != "/opt/app" || gotOpts.Env["APP_ENV"] != "prod" || gotOpts.Timeout != 30*time.Second {
		t.Fatalf("opts = %+v", gotOpts)
	}
}

func TestBatchRunWritesLogsPerHost(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveStore(&core.Store{Hosts: []core.Host{
		{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22},
		{Name: "web-2", IP: "10.0.0.9", User: "root", Password: "secret", Port: 22},
	}}); err != nil {
		t.Fatalf("save store: %v", err)
	}
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"batch-run", "--hosts", "devhost,web-2", "--parallel", "1", "--", "hostname"}); err != nil {
		t.Fatalf("batch-run: %v", err)
	}
	for _, target := range []string{"devhost", "web-2"} {
		lines, err := core.ReadRunLogs(target, "", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) != 1 || !strings.Contains(lines[0], `"task_id":"`) || !strings.Contains(lines[0], `"command":"hostname"`) {
			t.Fatalf("%s logs = %#v", target, lines)
		}
	}
}

func TestBatchRunReturnsErrorWhenAnyHostFails(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveStore(&core.Store{Hosts: []core.Host{
		{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22},
		{Name: "web-2", IP: "10.0.0.9", User: "root", Password: "secret", Port: 22},
	}}); err != nil {
		t.Fatalf("save store: %v", err)
	}
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		if host.Name == "web-2" {
			return []byte("bad\n"), errors.New("exit status 1")
		}
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"batch-run", "--hosts", "devhost,web-2", "--parallel", "1", "--", "hostname"}); err == nil {
		t.Fatal("expected batch-run error")
	}
}

func TestBatchRunRejectsInvalidParallel(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveStore(&core.Store{Hosts: []core.Host{{
		Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22,
	}}}); err != nil {
		t.Fatalf("save store: %v", err)
	}
	app := newTestApp()
	err := app.RunWithArgs([]string{"batch-run", "--hosts", "devhost", "--parallel", "0", "--", "hostname"})
	if err == nil || !strings.Contains(err.Error(), "--parallel") {
		t.Fatalf("err = %v", err)
	}
}

func TestBatchRunRunsWithParallelLimit(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveStore(&core.Store{Hosts: []core.Host{
		{Name: "h1", IP: "10.0.0.1", User: "root", Password: "secret", Port: 22},
		{Name: "h2", IP: "10.0.0.2", User: "root", Password: "secret", Port: 22},
		{Name: "h3", IP: "10.0.0.3", User: "root", Password: "secret", Port: 22},
	}}); err != nil {
		t.Fatalf("save store: %v", err)
	}
	var running int32
	var maxRunning int32
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		current := atomic.AddInt32(&running, 1)
		for {
			max := atomic.LoadInt32(&maxRunning)
			if current <= max || atomic.CompareAndSwapInt32(&maxRunning, max, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"batch-run", "--hosts", "h1,h2,h3", "--parallel", "2", "--", "hostname"}); err != nil {
		t.Fatalf("batch-run: %v", err)
	}
	if max := atomic.LoadInt32(&maxRunning); max > 2 {
		t.Fatalf("max running = %d, want <= 2", max)
	}
}

func TestBatchRunFailFastSkipsPendingHosts(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveStore(&core.Store{Hosts: []core.Host{
		{Name: "h1", IP: "10.0.0.1", User: "root", Password: "secret", Port: 22},
		{Name: "h2", IP: "10.0.0.2", User: "root", Password: "secret", Port: 22},
		{Name: "h3", IP: "10.0.0.3", User: "root", Password: "secret", Port: 22},
	}}); err != nil {
		t.Fatalf("save store: %v", err)
	}
	var calls int32
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return []byte("bad\n"), errors.New("exit status 1")
	}))
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))

	app := newTestApp()
	err := app.RunWithArgs([]string{"batch-run", "--hosts", "h1,h2,h3", "--parallel", "1", "--fail-fast", "--", "hostname"})
	if err == nil {
		t.Fatal("expected batch-run error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if !strings.Contains(out.String(), "skipped=2") {
		t.Fatalf("summary missing skipped count: %q", out.String())
	}
}

func TestBatchRunHostsFileRawIPsUsesSharedAuth(t *testing.T) {
	withTempConfig(t)
	path := filepath.Join(t.TempDir(), "ips.txt")
	if err := os.WriteFile(path, []byte("10.0.0.8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", Password: "secret"}},
	}); err != nil {
		t.Fatal(err)
	}

	var gotHost core.Host
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		gotHost = host
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"batch-run", "--hosts-file", path, "--auth", "dev-root", "--", "hostname"}); err != nil {
		t.Fatalf("batch-run raw: %v", err)
	}
	if gotHost.IP != "10.0.0.8" || gotHost.User != "root" || gotHost.Password != "secret" {
		t.Fatalf("host = %+v", gotHost)
	}
}

func TestBatchRunPasswordPromptReadsOnce(t *testing.T) {
	withTempConfig(t)
	path := filepath.Join(t.TempDir(), "ips.txt")
	if err := os.WriteFile(path, []byte("10.0.0.8\n10.0.0.9\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := core.SaveConfig(&core.Config{Defaults: core.Defaults{User: "root"}}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	t.Cleanup(setReadInteractivePasswordForTest(func(question ...string) string {
		calls++
		return "secret"
	}))
	t.Cleanup(setRunRemoteForTest(func(host core.Host, command string, opts core.RunOptions) ([]byte, error) {
		if host.Password != "secret" {
			t.Fatalf("password = %q", host.Password)
		}
		return []byte("ok\n"), nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"batch-run", "--hosts-file", path, "-u", "root", "-p", "--", "hostname"}); err != nil {
		t.Fatalf("batch-run password prompt: %v", err)
	}
	if calls != 1 {
		t.Fatalf("password prompt calls = %d, want 1", calls)
	}
}
