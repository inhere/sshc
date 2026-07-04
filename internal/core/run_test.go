package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
