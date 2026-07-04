package command

import (
	"fmt"
	"strings"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/gcli/v3"
)

var logOpts = struct {
	Match string
	Tail  int
}{Tail: 200}

func NewLogCmd() *gcli.Command {
	cmd := &gcli.Command{
		Name:    "log",
		Desc:    "show or search run logs",
		Aliases: []string{"logs"},
		Help: strings.TrimSpace(`
Examples:
  sshc log
  sshc log devhost
  sshc log 192.168.1.10
  sshc log devhost --match uptime
  sshc log devhost -m error --tail 50

Log files:
  ~/.config/sshc/logs/<host>.log

Notes:
  - Configure logs_path in sshc.config.json to use another log directory.
  - Without target, all host log files are read in file-name order.
  - With target, sshc resolves a saved host first, so IP can map to the host name log.
`),
		Config: func(c *gcli.Command) {
			c.StrOpt(&logOpts.Match, "match", "m", "", "match log lines by keyword")
			c.IntOpt(&logOpts.Tail, "tail", "", 200, "max lines to print")
			c.AddArg("target", "host ip or name, empty means all logs", false)
		},
		Func: func(c *gcli.Command, _ []string) error {
			target := strings.TrimSpace(c.Arg("target").String())
			logTarget, err := resolveLogTarget(target)
			if err != nil {
				return err
			}
			lines, err := core.ReadRunLogs(logTarget, logOpts.Match, logOpts.Tail)
			if err != nil {
				return err
			}
			for _, line := range lines {
				fmt.Fprintln(cmdOutput(c), line)
			}
			return nil
		},
	}
	return cmd
}

func resolveLogTarget(target string) (string, error) {
	if target == "" {
		return "", nil
	}
	store, err := core.LoadStoreWithSSHConfig()
	if err != nil {
		return "", err
	}
	if host, ok, err := store.ResolveHost(target); err != nil {
		return "", err
	} else if ok {
		return core.HostLogName(host), nil
	}
	return target, nil
}
