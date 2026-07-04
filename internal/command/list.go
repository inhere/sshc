package command

import (
	"fmt"
	"net"
	"strings"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/cliui/show/table"
	"github.com/gookit/goutil/cflag/capp"
)

var listOpts = struct {
	ShowIP bool
}{}

func NewListCmd() *capp.Cmd {
	cmd := capp.NewCmd("list", "list saved ssh hosts", func(c *capp.Cmd) error {
		store, err := core.LoadStoreWithSSHConfig()
		if err != nil {
			return err
		}
		out := buildHostListTable(store.Hosts, listOpts.ShowIP)
		if out != "" {
			fmt.Fprint(c.Output(), out)
		}
		return nil
	})
	cmd.Aliases = []string{"ls"}
	cmd.LongHelp = strings.TrimSpace(`
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
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.BoolVar(&listOpts.ShowIP, "show-ip", false, "show full host IP address")
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
