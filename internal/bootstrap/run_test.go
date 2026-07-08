package bootstrap

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/inhere/sshc/internal/core"
)

func TestRunInitializesRuntimeLogger(t *testing.T) {
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })

	dir := t.TempDir()
	t.Setenv(core.ConfigEnvKey, "")
	t.Setenv(core.ConfigDirEnvKey, dir)
	SetBuildInfo("test-version", "test-hash", "")

	code := Run([]string{"cfg", "path"})
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}

	data, err := os.ReadFile(filepath.Join(dir, "logs", "runtime", "sshc.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"msg":"command_start"`) || !strings.Contains(text, `"msg":"command_end"`) || !strings.Contains(text, `"version":"test-version"`) {
		t.Fatalf("runtime log = %s", text)
	}
}
