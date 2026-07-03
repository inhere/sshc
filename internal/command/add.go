package command

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"sshc/internal/core"

	"github.com/gookit/cliui/cutypes"
	"github.com/gookit/goutil/cflag/capp"
)

var addOpts = struct {
	Interactive bool
	IP          string
	Name        string
	User        string
	Password    string
	KeyPath     string
	Remark      string
	Group       string
	Port        int
}{Port: core.DefaultSSHPort}

func NewAddCmd() *capp.Cmd {
	cmd := capp.NewCmd("add", "add or update an ssh host", func(c *capp.Cmd) error {
		if addOpts.Port == 0 {
			addOpts.Port = core.DefaultSSHPort
		}

		var (
			host core.Host
			err  error
		)
		if addOpts.Interactive {
			host, err = collectInteractiveHost(cutypes.Input, c.Output())
		} else {
			host, err = buildHostFromAddOptions()
		}
		if err != nil {
			return err
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
  sshc add -I
  sshc add --ip 192.168.1.10 --name devhost -u root -p password --port 2222
  sshc add --ip 192.168.1.10 --name devhost -u root --key ~/.ssh/id_rsa
  sshc add --ip 192.168.1.10 --name devhost -u root -p password --remark "testing host" --group testing --key ~/.ssh/id_rsa

Notes:
  - If --name is empty, the IP is used as the host name.
  - If --group is empty, "default" is used.
  - Password or --key must be provided.
  - If both password and --key are provided, key authentication is tried first.
  - Adding the same name or IP updates the saved host.
  - Hosts are stored in ~/.config/sshc/hosts.json by default.
  - Passwords are currently stored in plain text. Keep the config file private.
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.BoolVar(&addOpts.Interactive, "interactive", false, "interactive host entry;;I")
		c.StringVar(&addOpts.IP, "ip", "", "ssh host ip or hostname")
		c.StringVar(&addOpts.Name, "name", "", "host alias")
		c.StringVar(&addOpts.User, "user", "", "ssh username;;u")
		c.StringVar(&addOpts.Password, "password", "", "ssh password;;p")
		c.StringVar(&addOpts.KeyPath, "key", "", "ssh private key path")
		c.StringVar(&addOpts.Remark, "remark", "", "host remark")
		c.StringVar(&addOpts.Group, "group", core.DefaultGroup, "host group")
		c.IntVar(&addOpts.Port, "port", core.DefaultSSHPort, "ssh port")
	}
	return cmd
}

func buildHostFromAddOptions() (core.Host, error) {
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
	normalizeHostDefaults(&host)
	return host, nil
}

func collectInteractiveHost(input io.Reader, output io.Writer) (core.Host, error) {
	if input == nil {
		input = cutypes.Input
	}
	if output == nil {
		output = cutypes.Output
	}
	reader := bufio.NewReader(input)
	host := core.Host{}
	var err error
	if host.Name, err = promptLine(reader, output, "Name", ""); err != nil {
		return host, err
	}
	if host.IP, err = promptLine(reader, output, "IP/Host", ""); err != nil {
		return host, err
	}
	if host.User, err = promptLine(reader, output, "User", "root"); err != nil {
		return host, err
	}
	if host.Password, err = promptLine(reader, output, "Password", ""); err != nil {
		return host, err
	}
	if host.KeyPath, err = promptLine(reader, output, "Key path", ""); err != nil {
		return host, err
	}
	portText, err := promptLine(reader, output, "Port", strconv.Itoa(core.DefaultSSHPort))
	if err != nil {
		return host, err
	}
	host.Port, err = strconv.Atoi(portText)
	if err != nil {
		return host, fmt.Errorf("invalid ssh port %q", portText)
	}
	if host.Remark, err = promptLine(reader, output, "Remark", ""); err != nil {
		return host, err
	}
	if host.Group, err = promptLine(reader, output, "Group", core.DefaultGroup); err != nil {
		return host, err
	}
	normalizeHostDefaults(&host)
	return host, nil
}

func promptLine(reader *bufio.Reader, output io.Writer, label, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(output, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(output, "%s: ", label)
	}
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		value = defaultValue
	}
	return value, nil
}

func normalizeHostDefaults(host *core.Host) {
	host.Name = strings.TrimSpace(host.Name)
	host.IP = strings.TrimSpace(host.IP)
	host.User = strings.TrimSpace(host.User)
	host.KeyPath = strings.TrimSpace(host.KeyPath)
	host.Remark = strings.TrimSpace(host.Remark)
	host.Group = strings.TrimSpace(host.Group)
	if host.Name == "" {
		host.Name = host.IP
	}
	if host.Group == "" {
		host.Group = core.DefaultGroup
	}
}
