package command

import (
	"fmt"
	"strings"

	"sshc/internal/core"

	"github.com/gookit/goutil/cflag/capp"
)

func NewListCmd() *capp.Cmd {
	cmd := capp.NewCmd("list", "list saved ssh hosts", func(c *capp.Cmd) error {
		store, err := core.LoadStore()
		if err != nil {
			return err
		}
		for _, host := range store.Hosts {
			name := host.Name
			if name == "" {
				name = host.IP
			}
			fmt.Fprintf(c.Output(), "%s\t%s@%s:%d\n", name, host.User, host.IP, host.Port)
		}
		return nil
	})
	cmd.Aliases = []string{"ls"}
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc list
  sshc ls

Output:
  name    user@ip:port

Notes:
  - Hosts are read from ~/.config/sshc/hosts.json by default.
  - Set SSHC_CONFIG to use a different hosts file.
`)
	return cmd
}
