package command

import (
	"fmt"
	"strings"

	"sshc/internal/core"

	"github.com/gookit/goutil/cflag/capp"
)

func NewListCmd() *capp.Cmd {
	cmd := capp.NewCmd("list", "list saved ssh hosts", func(c *capp.Cmd) error {
		store, err := core.LoadStoreWithSSHConfig()
		if err != nil {
			return err
		}
		for _, host := range store.Hosts {
			name := host.Name
			if name == "" {
				name = host.IP
			}
			auth := "password"
			if strings.TrimSpace(host.KeyPath) != "" {
				auth = "key:" + strings.TrimSpace(host.KeyPath)
			}
			remark := strings.TrimSpace(host.Remark)
			if remark == "" {
				remark = "-"
			}
			fmt.Fprintf(c.Output(), "%s\t%s\t%s@%s:%d\t%s\t%s\n", name, core.HostGroupName(host), host.User, host.IP, host.Port, auth, remark)
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
