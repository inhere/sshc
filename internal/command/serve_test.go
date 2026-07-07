package command

import (
	"strings"
	"testing"
)

func TestServeHelp(t *testing.T) {
	app := newTestApp()

	if err := app.RunWithArgs([]string{"serve", "--help"}); err != nil {
		t.Fatalf("serve --help: %v", err)
	}
	help := NewServeCmd().Help
	for _, want := range []string{"sshc serve", "--no-open", "--readonly", "--web-dir", "--token"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help text missing %q:\n%s", want, help)
		}
	}
}
