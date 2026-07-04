package command

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/cliui/cutypes"
	"github.com/gookit/gcli/v3"
	"github.com/gookit/goutil/x/termenv"
	"golang.org/x/term"
)

var addOpts = struct {
	Interactive   bool
	FromClipboard bool
	IP            string
	Name          string
	User          string
	Password      string
	KeyPath       string
	Remark        string
	Group         string
	Port          int
	AuthRef       string
	Jump          string
}{Port: core.DefaultSSHPort}

func NewAddCmd() *gcli.Command {
	cmd := &gcli.Command{
		Name:    "add",
		Desc:    "add or update an ssh host",
		Aliases: []string{"set"},
		Help: strings.TrimSpace(`
Examples:
  sshc add --ip 192.168.1.10 -u root -p password
  sshc add -I
  sshc add --from-clipboard
  sshc add --ip 192.168.1.10 --name devhost -u root -p password --port 2222
  sshc add --ip 192.168.1.10 --name devhost -u root --key ~/.ssh/id_rsa
  sshc add --ip 192.168.1.10 --name devhost -u root -p password --remark "testing host" --group testing --key ~/.ssh/id_rsa

Notes:
  - If --name is empty, the IP is used as the host name.
  - If --group is empty, "default" is used.
  - Password or --key must be provided.
  - If both password and --key are provided, key authentication is tried first.
  - --from-clipboard accepts key=value lines or one line: ip,user,password,name,port.
  - Adding the same name or IP updates the saved host.
  - Hosts are stored in ~/.config/sshc/sshc.config.json by default.
  - Passwords are encrypted before saving to sshc.config.json.
  - The local encryption key is stored at ~/.config/sshc/key.
`),
		Config: func(c *gcli.Command) {
			c.BoolOpt(&addOpts.Interactive, "interactive", "I", false, "interactive host entry")
			c.BoolOpt(&addOpts.FromClipboard, "from-clipboard", "", false, "read host fields from clipboard")
			c.StrOpt(&addOpts.IP, "ip", "", "", "ssh host ip or hostname")
			c.StrOpt(&addOpts.Name, "name", "", "", "host alias")
			c.StrOpt(&addOpts.User, "user", "u", "", "ssh username")
			c.StrOpt(&addOpts.Password, "password", "p", "", "ssh password")
			c.StrOpt(&addOpts.KeyPath, "key", "", "", "ssh private key path")
			c.StrOpt(&addOpts.AuthRef, "auth", "", "", "auth profile name")
			c.StrOpt(&addOpts.Jump, "jump", "", "", "jump host name or ip")
			c.StrOpt(&addOpts.Remark, "remark", "", "", "host remark")
			c.StrOpt(&addOpts.Group, "group", "", core.DefaultGroup, "host group")
			c.IntOpt(&addOpts.Port, "port", "", core.DefaultSSHPort, "ssh port")
		},
		Func: func(c *gcli.Command, _ []string) error {
			if addOpts.Port == 0 {
				addOpts.Port = core.DefaultSSHPort
			}
			if addOpts.Interactive && addOpts.FromClipboard {
				return fmt.Errorf("--interactive and --from-clipboard cannot be used together")
			}

			var (
				host core.Host
				err  error
			)
			if addOpts.Interactive {
				host, err = collectInteractiveHost(cutypes.Input, cmdOutput(c))
			} else if addOpts.FromClipboard {
				text, readErr := readClipboard()
				if readErr != nil {
					return readErr
				}
				host, err = parseClipboardHost(text)
			} else {
				host, err = buildHostFromAddOptions()
			}
			if err != nil {
				return err
			}

			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			store := core.Store{LogsPath: config.LogsPath, Hosts: config.Hosts}
			if err := store.Upsert(host); err != nil {
				return err
			}
			config.Hosts = store.Hosts
			if err := core.SaveConfig(config); err != nil {
				return err
			}

			fmt.Fprintf(cmdOutput(c), "saved %s (%s:%d)\n", host.Name, host.IP, host.Port)
			return nil
		},
	}
	return cmd
}

var (
	readClipboard           = readSystemClipboard
	readInteractivePassword = termenv.ReadPassword
)

func buildHostFromAddOptions() (core.Host, error) {
	host := core.Host{
		Name:     strings.TrimSpace(addOpts.Name),
		IP:       strings.TrimSpace(addOpts.IP),
		User:     strings.TrimSpace(addOpts.User),
		Password: addOpts.Password,
		KeyPath:  strings.TrimSpace(addOpts.KeyPath),
		AuthRef:  strings.TrimSpace(addOpts.AuthRef),
		Jump:     strings.TrimSpace(addOpts.Jump),
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
	if host.Password, err = promptPassword(reader, output, shouldReadHiddenPassword(input)); err != nil {
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
	if host.Jump, err = promptLine(reader, output, "Jump", ""); err != nil {
		return host, err
	}
	normalizeHostDefaults(&host)
	return host, nil
}

func promptPassword(reader *bufio.Reader, output io.Writer, hidden bool) (string, error) {
	if hidden {
		return strings.TrimSpace(readInteractivePassword("Password: ")), nil
	}
	return promptLine(reader, output, "Password", "")
}

func shouldReadHiddenPassword(input io.Reader) bool {
	file, ok := input.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
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
	host.Jump = strings.TrimSpace(host.Jump)
	host.Remark = strings.TrimSpace(host.Remark)
	host.Group = strings.TrimSpace(host.Group)
	if host.Name == "" {
		host.Name = host.IP
	}
	if host.Group == "" {
		host.Group = core.DefaultGroup
	}
}
