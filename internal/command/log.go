package command

import (
	"errors"
	"fmt"
	"strings"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/gcli/v3"
)

var logOpts = struct {
	Match string
	Tail  int
	ID    string
	Lines string
}{Tail: 200}

func NewLogCmd() *gcli.Command {
	logOpts = struct {
		Match string
		Tail  int
		ID    string
		Lines string
	}{Tail: 200}

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
  sshc log --id 20260704-173012-a1b2c3
  sshc log --id 20260704-173012-a1b2c3 --tail 80
  sshc log --id 20260704-173012-a1b2c3 --lines 120,180
  sshc log devhost --lines 20,80

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
			c.StrOpt(&logOpts.ID, "id", "", "", "show output for task id")
			c.StrOpt(&logOpts.Lines, "lines", "", "", "line range start,end")
			c.AddArg("target", "host ip or name, empty means all logs", false)
		},
		Func: func(c *gcli.Command, _ []string) error {
			if err := validateLogOptions(); err != nil {
				return err
			}
			target := strings.TrimSpace(c.Arg("target").String())
			logTarget, err := resolveLogTarget(target)
			if err != nil {
				return err
			}
			if strings.TrimSpace(logOpts.ID) != "" {
				return printLogOutputByID(c, logTarget)
			}
			lines, err := core.ReadRunLogsSelected(logTarget, logOpts.Match, logOpts.Tail, logOpts.Lines)
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

func validateLogOptions() error {
	if strings.TrimSpace(logOpts.ID) != "" && strings.TrimSpace(logOpts.Match) != "" {
		return errors.New("--id and --match cannot be used together")
	}
	if strings.TrimSpace(logOpts.Lines) != "" && logOpts.Tail != 200 {
		return errors.New("--lines and --tail cannot be used together")
	}
	return nil
}

func printLogOutputByID(c *gcli.Command, target string) error {
	output, err := core.ReadRunLogOutputByID(target, logOpts.ID)
	if err != nil {
		return err
	}
	lines := splitLogOutputLines(string(output))
	lines, err = core.SelectLogLines(lines, logOpts.Lines, logOpts.Tail)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(cmdOutput(c), line)
	}
	return nil
}

func splitLogOutputLines(output string) []string {
	output = strings.TrimRight(output, "\r\n")
	if output == "" {
		return nil
	}
	return strings.Split(output, "\n")
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
