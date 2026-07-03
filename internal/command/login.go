package command

import (
	"fmt"
	"strings"

	"sshc/internal/core"

	"github.com/gookit/goutil/cflag/capp"
)

var loginRemote = core.LoginRemote

func NewLoginCmd() *capp.Cmd {
	cmd := capp.NewCmd("login", "connect to a remote shell", func(c *capp.Cmd) error {
		target := strings.TrimSpace(c.Arg("target").String())
		store, err := core.LoadStoreWithSSHConfig()
		if err != nil {
			return err
		}
		host, ok, err := store.ResolveHost(target)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("host %q not found", target)
		}

		startedAt := core.Now()
		err = loginRemote(host)
		logErr := core.AppendRunLog(host, core.RunLogRecord{
			Target:     target,
			Command:    "login",
			Status:     core.RunStatus(err),
			StartedAt:  startedAt,
			DurationMS: core.SinceMS(startedAt),
			Error:      core.ErrorString(err),
		})
		if err == nil && logErr != nil {
			return logErr
		}
		return err
	})
	cmd.Aliases = []string{"connect"}
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc login devhost
  sshc connect devhost

Notes:
  - Opens an interactive PTY shell on the remote host.
  - By default, sshc only logs connection metadata, not full session input/output.
  - Use run for non-interactive commands that need full stdout/stderr logs.
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.AddArg("target", "host ip or name", true)
	}
	return cmd
}
