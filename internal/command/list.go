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
	cmd := newHostListCmd()
	cmd.Help = `
Examples:
  sshc list --show-ip
  sshc list --tag testing

Output:
  name    group    tags    user@ip:port    auth    remark

Notes:
  - IPv4 addresses are masked by default, for example 10.*.*.8.
  - Use --show-ip to print full IP addresses.
  - Hosts are read from ~/.config/sshc/sshc.config.json by default.
  - Set SSHC_CONFIG to use a different hosts file.
`
	return cmd
}

func buildHostListTable(hosts []core.Host, showIP bool) string {
	if len(hosts) == 0 {
		return ""
	}
	tb := table.New("", table.WithBorderFlags(table.BorderDefault), table.WithOverflowFlag(table.OverflowWrap))
	tb.SetHeads("Name", "Group", "Tags", "Address", "Auth", "Remark")
	for _, host := range hosts {
		name := host.Name
		if name == "" {
			name = host.IP
		}
		remark := strings.TrimSpace(host.Remark)
		if remark == "" {
			remark = "-"
		}
		address := fmt.Sprintf("%s@%s:%d", host.User, displayHostIP(host.IP, showIP), host.Port)
		auth := core.AuthLabel(host)
		if core.IsCommandProxyHost(host) {
			address = fmt.Sprintf("via:%s", strings.TrimSpace(host.Via))
			auth = core.HostBackendCommandProxy
		}
		tb.AddRow(name, core.HostGroupName(host), core.HostTagsLabel(host), address, auth, remark)
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
