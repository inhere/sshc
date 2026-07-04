package command

import (
	"testing"

	"github.com/inhere/sshc/internal/core"
)

func TestLoginPassesJumpOption(t *testing.T) {
	saveJumpCommandHosts(t)

	var gotHost core.Host
	t.Cleanup(setLoginRemoteForTest(func(host core.Host, opts core.LoginOptions) error {
		gotHost = host
		return nil
	}))

	app := newTestApp()
	if err := app.RunWithArgs([]string{"login", "inner-db", "--jump", "bastion"}); err != nil {
		t.Fatalf("login with jump: %v", err)
	}
	if gotHost.Jump != "bastion" {
		t.Fatalf("jump = %q, want bastion", gotHost.Jump)
	}
}
