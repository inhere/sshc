package command

import (
	"fmt"
	"strings"

	"sshc/internal/core"

	"github.com/gookit/goutil/cflag/capp"
)

var addOpts = struct {
	IP       string
	Name     string
	User     string
	Password string
	KeyPath  string
	Remark   string
	Group    string
	Port     int
}{Port: core.DefaultSSHPort}

func NewAddCmd() *capp.Cmd {
	cmd := capp.NewCmd("add", "add or update an ssh host", func(c *capp.Cmd) error {
		if addOpts.Port == 0 {
			addOpts.Port = core.DefaultSSHPort
		}

		host := core.Host{
			Name:     strings.TrimSpace(addOpts.Name),
			IP:       strings.TrimSpace(addOpts.IP),
			User:     strings.TrimSpace(addOpts.User),
			Password: addOpts.Password,
			KeyPath:  strings.TrimSpace(addOpts.KeyPath),
			Remark:   strings.TrimSpace(addOpts.Remark),
			Group:    strings.TrimSpace(addOpts.Group),
			Port:     addOpts.Port,
		}
		if host.Name == "" {
			host.Name = host.IP
		}
		if host.Group == "" {
			host.Group = core.DefaultGroup
		}

		store, err := core.LoadStore()
		if err != nil {
			return err
		}
		if err := store.Upsert(host); err != nil {
			return err
		}
		if err := core.SaveStore(store); err != nil {
			return err
		}

		fmt.Fprintf(c.Output(), "saved %s (%s:%d)\n", host.Name, host.IP, host.Port)
		return nil
	})
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc add --ip 192.168.1.10 -u root -p password
  sshc add --ip 192.168.1.10 --name devhost -u root -p password --port 2222
  sshc add --ip 192.168.1.10 --name devhost -u root -p password --remark "testing host" --group testing --key ~/.ssh/id_rsa

Notes:
  - If --name is empty, the IP is used as the host name.
  - If --group is empty, "default" is used.
  - Adding the same name or IP updates the saved host.
  - Hosts are stored in ~/.config/sshc/hosts.json by default.
  - Passwords are currently stored in plain text. Keep the config file private.
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&addOpts.IP, "ip", "", "ssh host ip or hostname;true")
		c.StringVar(&addOpts.Name, "name", "", "host alias")
		c.StringVar(&addOpts.User, "user", "", "ssh username;true;u")
		c.StringVar(&addOpts.Password, "password", "", "ssh password;true;p")
		c.StringVar(&addOpts.KeyPath, "key", "", "ssh private key path")
		c.StringVar(&addOpts.Remark, "remark", "", "host remark")
		c.StringVar(&addOpts.Group, "group", core.DefaultGroup, "host group")
		c.IntVar(&addOpts.Port, "port", core.DefaultSSHPort, "ssh port")
	}
	return cmd
}
