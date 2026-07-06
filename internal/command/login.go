package command

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/cliui/cutypes"
	"github.com/gookit/cliui/interact"
	"github.com/gookit/gcli/v3"
)

var loginRemote = core.LoginRemoteWithOptions
var selectLoginHost = selectLoginHostInteractive

func NewLoginCmd() *gcli.Command {
	var termName string
	var jumpName string

	cmd := &gcli.Command{
		Name:    "login",
		Desc:    "connect to a remote shell",
		Aliases: []string{"connect"},
		Help: strings.TrimSpace(`
Examples:
  sshc login
  sshc login devhost
  sshc connect devhost
  sshc login inner-db --jump bastion
  sshc login devhost --term xterm-256color

Notes:
  - Terminal resize is forwarded on Unix-like systems; Windows uses the startup terminal size.
  - By default, sshc only logs connection metadata, not full session input/output.
  - Use run for non-interactive commands that need full stdout/stderr logs.
`),
		Config: func(c *gcli.Command) {
			c.AddArg("target", "host ip or name", false)
			c.StrOpt(&jumpName, "jump", "", "", "jump host name or ip")
			c.StrOpt(&termName, "term", "", "", "remote terminal type, defaults to TERM or xterm-256color")
		},
		Func: func(c *gcli.Command, _ []string) error {
			target := strings.TrimSpace(c.Arg("target").String())
			host, selectedTarget, err := resolveLoginHost(target, core.ResolveConnectionOptions{Jump: jumpName}, c)
			if err != nil {
				return err
			}

			startedAt := core.Now()
			fmt.Fprintln(cmdOutput(c), loginConnectMessage(host))
			logBackend, logVia, proxiedCommand := commandProxyLoginLogFields(host)
			err = loginRemote(host, core.LoginOptions{Term: termName})
			logErr := core.AppendRunLog(host, core.RunLogRecord{
				Target:         selectedTarget,
				Command:        "login",
				Status:         core.RunStatus(err),
				StartedAt:      startedAt,
				DurationMS:     core.SinceMS(startedAt),
				Error:          core.ErrorString(err),
				Backend:        logBackend,
				Via:            logVia,
				ProxiedCommand: proxiedCommand,
			})
			if err == nil && logErr != nil {
				return logErr
			}
			return err
		},
	}
	return cmd
}

func resolveLoginHost(target string, opts core.ResolveConnectionOptions, c *gcli.Command) (core.Host, string, error) {
	target = strings.TrimSpace(target)
	config, err := core.LoadConfigWithSSHConfig()
	if err != nil {
		return core.Host{}, "", err
	}
	store := core.Store{LogsPath: config.LogsPath, Hosts: config.Hosts}

	if target != "" {
		host, ok, err := store.ResolveHost(target)
		if err == nil && ok {
			return effectiveLoginHost(config, host, opts, target)
		}
		if err != nil && len(store.MatchHosts(target)) == 0 {
			return core.Host{}, "", err
		}
	}

	candidates := store.Hosts
	if target != "" {
		if matches := store.MatchHosts(target); len(matches) > 0 {
			candidates = matches
		} else {
			fmt.Fprintf(cmdOutput(c), "host %q not found; select a host\n", target)
		}
	}
	if len(candidates) == 0 {
		return core.Host{}, "", fmt.Errorf("no hosts available; use \"sshc add -I\" to add one")
	}

	selected, err := selectLoginHost(candidates, cutypes.Input, cmdOutput(c))
	if err != nil {
		return core.Host{}, "", err
	}
	selectedTarget := core.HostLogName(selected)
	if selectedTarget == "" {
		selectedTarget = selected.IP
	}
	return effectiveLoginHost(config, selected, opts, selectedTarget)
}

func effectiveLoginHost(config *core.Config, host core.Host, opts core.ResolveConnectionOptions, selectedTarget string) (core.Host, string, error) {
	effective, _, err := config.EffectiveHost(host, core.HostOverrides{})
	if err != nil {
		return core.Host{}, "", err
	}
	resolved := effective.ToHost()
	if core.IsCommandProxyHost(resolved) && strings.TrimSpace(opts.Jump) != "" {
		return core.Host{}, "", fmt.Errorf("--jump is not supported for command_proxy targets; configure jump on the via host")
	}
	if jump := strings.TrimSpace(opts.Jump); jump != "" {
		resolved.Jump = jump
	}
	return resolved, selectedTarget, nil
}

func selectLoginHostInteractive(hosts []core.Host, input io.Reader, output io.Writer) (core.Host, error) {
	if input == nil {
		input = cutypes.Input
	}
	if output == nil {
		output = cutypes.Output
	}
	items := make([]interact.UIItem, 0, len(hosts))
	for i, host := range hosts {
		items = append(items, interact.UIItem{
			Key:   strconv.Itoa(i + 1),
			Label: formatLoginHostChoice(host),
			Value: host,
		})
	}
	selector := interact.NewUISelect("Select host", items)
	selector.Filterable = true
	selector.PageSize = 10
	result, err := selector.RunWithIO(context.Background(), interact.NewUIReadlineBackend(), input, output)
	if err != nil {
		return core.Host{}, err
	}
	host, ok := result.Value.(core.Host)
	if !ok {
		return core.Host{}, fmt.Errorf("invalid selected host")
	}
	return host, nil
}

func formatLoginHostChoice(host core.Host) string {
	remark := strings.TrimSpace(host.Remark)
	if remark == "" {
		remark = "-"
	}
	if core.IsCommandProxyHost(host) {
		return fmt.Sprintf("%s  %s  command_proxy via:%s  %s",
			core.HostLogName(host),
			core.HostGroupName(host),
			strings.TrimSpace(host.Via),
			remark,
		)
	}
	return fmt.Sprintf("%s  %s  %s@%s:%d  %s  %s",
		core.HostLogName(host),
		core.HostGroupName(host),
		host.User,
		displayHostIP(host.IP, false),
		host.Port,
		core.AuthLabel(host),
		remark,
	)
}

func loginConnectMessage(host core.Host) string {
	if core.IsCommandProxyHost(host) {
		return fmt.Sprintf("connecting to %s (command_proxy via:%s)", core.HostLogName(host), strings.TrimSpace(host.Via))
	}
	return fmt.Sprintf("connecting to %s (%s@%s:%d)", core.HostLogName(host), host.User, host.IP, host.Port)
}

func commandProxyLoginLogFields(host core.Host) (backend, via, proxiedCommand string) {
	if !core.IsCommandProxyHost(host) {
		return "", "", ""
	}
	backend = core.HostBackendCommandProxy
	via = strings.TrimSpace(host.Via)
	plan, err := core.PlanCommandProxyLogin(host)
	if err != nil {
		return backend, via, strings.TrimSpace(host.LoginCommand)
	}
	return backend, core.HostLogName(plan.Via), plan.LoginCommand
}

func setSelectLoginHostForTest(fn func([]core.Host, io.Reader, io.Writer) (core.Host, error)) func() {
	old := selectLoginHost
	selectLoginHost = fn
	return func() { selectLoginHost = old }
}
