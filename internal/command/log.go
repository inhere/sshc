package command

import (
	"fmt"
	"strings"

	"sshc/internal/core"

	"github.com/gookit/goutil/cflag/capp"
)

var logOpts = struct {
	Match string
	Tail  int
}{Tail: 200}

func NewLogCmd() *capp.Cmd {
	cmd := capp.NewCmd("log", "show or search run logs", func(c *capp.Cmd) error {
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
			fmt.Fprintln(c.Output(), line)
		}
		return nil
	})
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc log
  sshc log devhost
  sshc log 192.168.1.10
  sshc log devhost --match uptime
  sshc log devhost -m error --tail 50

Log files:
  ~/.config/sshc/logs/<host>.log

Notes:
  - Without target, all host log files are read in file-name order.
  - With target, sshc resolves a saved host first, so IP can map to the host name log.
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&logOpts.Match, "match", "", "match log lines by keyword;;m")
		c.IntVar(&logOpts.Tail, "tail", 200, "max lines to print")
		c.AddArg("target", "host ip or name, empty means all logs", false)
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
