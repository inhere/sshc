package command

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"sshc/internal/core"

	"github.com/gookit/cliui/cutypes"
	"github.com/gookit/goutil/cflag/capp"
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
}{Port: core.DefaultSSHPort}

func NewAddCmd() *capp.Cmd {
	cmd := capp.NewCmd("add", "add or update an ssh host", func(c *capp.Cmd) error {
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
			host, err = collectInteractiveHost(cutypes.Input, c.Output())
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
  - Hosts are stored in ~/.config/sshc/hosts.json by default.
  - Passwords are currently stored in plain text. Keep the config file private.
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.BoolVar(&addOpts.Interactive, "interactive", false, "interactive host entry;;I")
		c.BoolVar(&addOpts.FromClipboard, "from-clipboard", false, "read host fields from clipboard")
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

var readClipboard = readSystemClipboard

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

func parseClipboardHost(text string) (core.Host, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return core.Host{}, fmt.Errorf("clipboard is empty")
	}
	var host core.Host
	if strings.Contains(text, "=") {
		host = parseClipboardKeyValues(text)
	} else {
		fields := strings.Split(text, ",")
		if len(fields) < 3 || len(fields) > 5 {
			return host, fmt.Errorf("clipboard must be key=value lines or ip,user,password,name,port")
		}
		host.IP = strings.TrimSpace(fields[0])
		host.User = strings.TrimSpace(fields[1])
		host.Password = strings.TrimSpace(fields[2])
		if len(fields) >= 4 {
			host.Name = strings.TrimSpace(fields[3])
		}
		if len(fields) >= 5 && strings.TrimSpace(fields[4]) != "" {
			port, err := strconv.Atoi(strings.TrimSpace(fields[4]))
			if err != nil {
				return host, fmt.Errorf("invalid ssh port %q", strings.TrimSpace(fields[4]))
			}
			host.Port = port
		}
	}
	if host.Port == 0 {
		host.Port = core.DefaultSSHPort
	}
	normalizeHostDefaults(&host)
	return host, nil
}

func parseClipboardKeyValues(text string) core.Host {
	host := core.Host{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "ip", "host", "hostname":
			host.IP = value
		case "name":
			host.Name = value
		case "user", "username":
			host.User = value
		case "password", "pwd":
			host.Password = value
		case "key", "key_path", "keypath":
			host.KeyPath = value
		case "remark":
			host.Remark = value
		case "group":
			host.Group = value
		case "port":
			if port, err := strconv.Atoi(value); err == nil {
				host.Port = port
			}
		}
	}
	return host
}

func readSystemClipboard() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw")
	case "darwin":
		cmd = exec.Command("pbpaste")
	default:
		if _, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.Command("wl-paste", "--no-newline")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else {
			return "", fmt.Errorf("no clipboard reader found; install wl-paste or xclip")
		}
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
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
