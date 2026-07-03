package command

import (
	"fmt"
	"strings"

	"sshc/internal/core"

	"github.com/gookit/cliui/show/table"
	"github.com/gookit/goutil/cflag/capp"
)

func NewListCmd() *capp.Cmd {
	cmd := capp.NewCmd("list", "list saved ssh hosts", func(c *capp.Cmd) error {
		store, err := core.LoadStoreWithSSHConfig()
		if err != nil {
			return err
		}
		out := buildHostListTable(store.Hosts)
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

Output:
  name    group    user@ip:port    auth    remark

Notes:
  - Hosts are read from ~/.config/sshc/hosts.json by default.
  - Set SSHC_CONFIG to use a different hosts file.
`)
	return cmd
}

func buildHostListTable(hosts []core.Host) string {
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
		tb.AddRow(name, core.HostGroupName(host), fmt.Sprintf("%s@%s:%d", host.User, host.IP, host.Port), hostAuthLabel(host), remark)
	}
	return tb.String()
}

func hostAuthLabel(host core.Host) string {
	if strings.TrimSpace(host.KeyPath) != "" {
		return "key:" + strings.TrimSpace(host.KeyPath)
	}
	return "password"
}
