package command

import (
	"fmt"
	"net"
	"strings"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/cliui/show/table"
	"github.com/gookit/gcli/v3"
)

var listOpts = struct {
	ShowIP bool
}{}

func NewListCmd() *gcli.Command {
	cmd := &gcli.Command{
		Name:    "list",
		Desc:    "list saved ssh hosts",
		Aliases: []string{"ls"},
		Help: strings.TrimSpace(`
Examples:
  sshc list
  sshc ls
  sshc list --show-ip

Output:
  name    group    user@ip:port    auth    remark

Notes:
  - IPv4 addresses are masked by default, for example 10.*.*.8.
  - Use --show-ip to print full IP addresses.
  - Hosts are read from ~/.config/sshc/sshc.config.json by default.
  - Set SSHC_CONFIG to use a different hosts file.
`),
		Config: func(c *gcli.Command) {
			c.BoolOpt(&listOpts.ShowIP, "show-ip", "", false, "show full host IP address")
		},
		Func: func(c *gcli.Command, _ []string) error {
			store, err := core.LoadStoreWithSSHConfig()
			if err != nil {
				return err
			}
			out := buildHostListTable(store.Hosts, listOpts.ShowIP)
			if out != "" {
				fmt.Fprint(cmdOutput(c), out)
			}
			return nil
		},
	}
	return cmd
}

func buildHostListTable(hosts []core.Host, showIP bool) string {
	if len(hosts) == 0 {
		return ""
	}
	tb := table.New("", table.WithBorderFlags(table.BorderDefault), table.WithOverflowFlag(table.OverflowWrap))
	tb.SetHeads("Name", "Group", "Address", "Auth", "Remark")
	for _, host := range hosts {
		name := host.Name
		if name == "" {
			name = host.IP
		}
		remark := strings.TrimSpace(host.Remark)
		if remark == "" {
			remark = "-"
		}
		tb.AddRow(name, core.HostGroupName(host), fmt.Sprintf("%s@%s:%d", host.User, displayHostIP(host.IP, showIP), host.Port), hostAuthLabel(host), remark)
	}
	return tb.String()
}

func displayHostIP(ip string, showIP bool) string {
	if showIP {
		return ip
	}
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return ip
	}
	ipv4 := parsed.To4()
	if ipv4 == nil {
		return ip
	}
	return fmt.Sprintf("%d.*.*.%d", ipv4[0], ipv4[3])
}

func hostAuthLabel(host core.Host) string {
	if strings.TrimSpace(host.KeyPath) != "" {
		return "key:" + strings.TrimSpace(host.KeyPath)
	}
	return "password"
}
