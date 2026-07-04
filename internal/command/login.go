package command

import (
	"fmt"
	"strings"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/gcli/v3"
)

var loginRemote = core.LoginRemoteWithOptions

func NewLoginCmd() *gcli.Command {
	var termName string

	cmd := &gcli.Command{
		Name:    "login",
		Desc:    "connect to a remote shell",
		Aliases: []string{"connect"},
		Help: strings.TrimSpace(`
Examples:
  sshc login devhost
  sshc connect devhost
  sshc login devhost --term xterm-256color

Notes:
  - Terminal resize is forwarded on Unix-like systems; Windows uses the startup terminal size.
  - By default, sshc only logs connection metadata, not full session input/output.
  - Use run for non-interactive commands that need full stdout/stderr logs.
`),
		Config: func(c *gcli.Command) {
			c.AddArg("target", "host ip or name", true)
			c.StrOpt(&termName, "term", "", "", "remote terminal type, defaults to TERM or xterm-256color")
		},
		Func: func(c *gcli.Command, _ []string) error {
			target := strings.TrimSpace(c.Arg("target").String())
			host, err := resolveCommandHost(target)
			if err != nil {
				return err
			}

			startedAt := core.Now()
			fmt.Fprintf(cmdOutput(c), "connecting to %s (%s@%s:%d)\n", core.HostLogName(host), host.User, host.IP, host.Port)
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
		},
	}
	return cmd
}
