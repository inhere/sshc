package command

import (
	"fmt"
	"strings"

	"sshc/internal/core"

	"github.com/gookit/goutil/cflag/capp"
)

var loginRemote = core.LoginRemoteWithOptions

func NewLoginCmd() *capp.Cmd {
	var termName string

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
		fmt.Fprintf(c.Output(), "connecting to %s (%s@%s:%d)\n", core.HostLogName(host), host.User, host.IP, host.Port)
		err = loginRemote(host, core.LoginOptions{Term: termName})
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
  sshc login devhost --term xterm-256color

Notes:
  - Terminal resize is forwarded on Unix-like systems; Windows uses the startup terminal size.
  - By default, sshc only logs connection metadata, not full session input/output.
  - Use run for non-interactive commands that need full stdout/stderr logs.
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.AddArg("target", "host ip or name", true)
		c.StringVar(&termName, "term", "", "remote terminal type, defaults to TERM or xterm-256color")
	}
	return cmd
}
