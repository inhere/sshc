package core

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeLogPathUsesConfiguredLogsPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(ConfigEnvKey, "")
	t.Setenv(ConfigDirEnvKey, dir)
	if err := SaveConfig(&Config{LogsPath: "logs", Hosts: []Host{}}); err != nil {
		t.Fatal(err)
	}

	path, err := RuntimeLogPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "logs", runtimeLogDirName, "sshc.log"); path != want {
		t.Fatalf("runtime log path = %q, want %q", path, want)
	}
}

func TestInitRuntimeLoggerWritesJSONL(t *testing.T) {
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })

	dir := t.TempDir()
	t.Setenv(ConfigEnvKey, "")
	t.Setenv(ConfigDirEnvKey, dir)
	if err := SaveConfig(&Config{LogsPath: "logs", Hosts: []Host{}}); err != nil {
		t.Fatal(err)
	}

	logger, err := InitRuntimeLogger()
	if err != nil {
		t.Fatal(err)
	}
	slog.Info("runtime_test", slog.String("key", "value"))
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "logs", runtimeLogDirName, "sshc.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"msg":"runtime_test"`) || !strings.Contains(text, `"key":"value"`) {
		t.Fatalf("runtime log = %s", text)
	}
}

func TestRuntimeLogArgsMasksSecrets(t *testing.T) {
	args := []string{
		"add", "--ip", "10.0.0.8", "-p", "secret",
		"--token=abc", "--key", "~/.ssh/id_rsa", "--password=plain", "-e", "API_TOKEN=token",
	}
	got := strings.Join(RuntimeLogArgs(args), " ")
	for _, secret := range []string{"secret", "abc", "~/.ssh/id_rsa", "plain", "API_TOKEN=token"} {
		if strings.Contains(got, secret) {
			t.Fatalf("args leaked %q: %s", secret, got)
		}
	}
	if !strings.Contains(got, "-p ***") || !strings.Contains(got, "--token=***") || !strings.Contains(got, "--key ***") || !strings.Contains(got, "--password=***") || !strings.Contains(got, "-e ***") {
		t.Fatalf("args not masked as expected: %s", got)
	}
}
