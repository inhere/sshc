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
	EmbedKey      bool
	KeyPassphrase keyPassphraseFlag
	Remark        string
	Group         string
	Tags          string
	Port          int
	AuthRef       string
	Jump          string
	Backend       string
	Via           string
	RunTemplate   string
	LoginCommand  string
}{Port: core.DefaultSSHPort}

func NewAddCmd() *gcli.Command {
	resetAddOptions()
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
  sshc add --ip 192.168.1.10 --name devhost -u root --key ~/.ssh/id_rsa --embed-key
  sshc add --ip 192.168.1.10 --name devhost -u root --key ~/.ssh/id_rsa --embed-key --key-passphrase
  sshc add --ip 192.168.1.10 --name devhost -u root --key ~/.ssh/id_rsa --key-passphrase=env
  sshc add --ip 192.168.1.10 --name devhost -u root -p password --remark "testing host" --group testing --tags app,testing --key ~/.ssh/id_rsa
  sshc add --ip 10.0.0.8 --name inner-db --auth dev-root --jump bastion
  sshc add --name lxc-app --backend command_proxy --via pve-host --run-template "pct exec 101 -- sh -lc {{cmd}}"

Notes:
  - If --name is empty, the IP is used as the host name.
  - If --group is empty, "default" is used.
  - Password or --key must be provided.
  - If both password and --key are provided, key authentication is tried first.
  - --embed-key stores encrypted private key content in sshc.config.json.
  - --key-passphrase reads a private key passphrase from hidden input by default.
  - --key-passphrase=clip reads it from clipboard; --key-passphrase=env reads ${SSHC_KEY_PASSPHRASE}.
  - --jump stores a default jump host for run/login/scp/download.
  - command_proxy hosts use --via and --run-template/--login-command instead of direct SSH fields.
  - --from-clipboard accepts key=value lines or one line: ip,user,password,name,port.
  - Adding the same name or IP updates the saved host.
  - Hosts are stored in ~/.config/sshc/sshc.config.json by default.
  - Passwords are encrypted before saving to sshc.config.json.
  - The local encryption key is stored at ~/.config/sshc/key.
`),
		Config: func(c *gcli.Command) {
			c.BoolOpt(&addOpts.Interactive, "interactive", "I", false, "interactive host entry")
			c.BoolOpt(&addOpts.FromClipboard, "from-clipboard", "fc", false, "read host fields from clipboard")
			c.StrOpt(&addOpts.IP, "ip", "", "", "ssh host ip or hostname")
			c.StrOpt(&addOpts.Name, "name", "", "", "host alias")
			c.StrOpt(&addOpts.User, "user", "u", "", "ssh username")
			c.StrOpt(&addOpts.Password, "password", "p", "", "ssh password")
			c.StrOpt(&addOpts.KeyPath, "key", "", "", "ssh private key path")
			c.BoolOpt(&addOpts.EmbedKey, "embed-key", "", false, "store encrypted private key content in config")
			c.VarOpt(&addOpts.KeyPassphrase, "key-passphrase", "", "key passphrase source: input, clip, or env")
			c.StrOpt(&addOpts.AuthRef, "auth", "", "", "auth profile name")
			c.StrOpt(&addOpts.Jump, "jump", "", "", "jump host name or ip")
			c.StrOpt(&addOpts.Backend, "backend", "", "", "host backend: ssh or command_proxy")
			c.StrOpt(&addOpts.Via, "via", "", "", "command_proxy via host name or ip")
			c.StrOpt(&addOpts.RunTemplate, "run-template", "", "", "command_proxy run template")
			c.StrOpt(&addOpts.LoginCommand, "login-command", "", "", "command_proxy login command")
			c.StrOpt(&addOpts.Remark, "remark", "", "", "host remark")
			c.StrOpt(&addOpts.Group, "group", "", core.DefaultGroup, "host group")
			c.StrOpt(&addOpts.Tags, "tags", "", "", "comma-separated host tags")
			c.IntOpt(&addOpts.Port, "port", "", core.DefaultSSHPort, "ssh port")
			c.AddArg("key_passphrase_source", "key passphrase source: input, clip, or env", false)
		},
		Func: func(c *gcli.Command, args []string) error {
			var err error
			if sourceArg := strings.TrimSpace(c.Arg("key_passphrase_source").String()); sourceArg != "" {
				if !addOpts.KeyPassphrase.SetFlag {
					return fmt.Errorf("unexpected argument %q", sourceArg)
				}
				if err := addOpts.KeyPassphrase.Set(sourceArg); err != nil {
					return err
				}
			}
			args, err = consumeKeyPassphraseSourceArg(&addOpts.KeyPassphrase, args)
			if err != nil {
				return err
			}
			if len(args) > 0 {
				return fmt.Errorf("unexpected argument %q", args[0])
			}
			if addOpts.Port == 0 {
				addOpts.Port = core.DefaultSSHPort
			}
			if addOpts.Interactive && addOpts.FromClipboard {
				return fmt.Errorf("--interactive and --from-clipboard cannot be used together")
			}

			var (
				host core.Host
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
			if host.KeyPath, err = normalizeKeyPathForSave(host.KeyPath); err != nil {
				return err
			}
			if err := applyHostKeyOptions(&host, addOpts.EmbedKey, addOpts.KeyPassphrase); err != nil {
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
			if issues := core.CheckConfig(*config); core.HasDoctorErrors(issues) {
				return fmt.Errorf("invalid host config: %s", formatDoctorErrors(issues))
			}
			if err := core.SaveConfig(config); err != nil {
				return err
			}

			fmt.Fprintf(cmdOutput(c), "saved %s (%s:%d)\n", host.Name, host.IP, host.Port)
			return nil
		},
	}
	return cmd
}

func resetAddOptions() {
	addOpts.Interactive = false
	addOpts.FromClipboard = false
	addOpts.IP = ""
	addOpts.Name = ""
	addOpts.User = ""
	addOpts.Password = ""
	addOpts.KeyPath = ""
	addOpts.EmbedKey = false
	addOpts.KeyPassphrase = keyPassphraseFlag{}
	addOpts.Remark = ""
	addOpts.Group = core.DefaultGroup
	addOpts.Tags = ""
	addOpts.Port = core.DefaultSSHPort
	addOpts.AuthRef = ""
	addOpts.Jump = ""
	addOpts.Backend = ""
	addOpts.Via = ""
	addOpts.RunTemplate = ""
	addOpts.LoginCommand = ""
}

var (
	readClipboard           = readSystemClipboard
	readInteractivePassword = termenv.ReadPassword
)

func buildHostFromAddOptions() (core.Host, error) {
	host := core.Host{
		Name:         strings.TrimSpace(addOpts.Name),
		IP:           strings.TrimSpace(addOpts.IP),
		User:         strings.TrimSpace(addOpts.User),
		Password:     addOpts.Password,
		KeyPath:      strings.TrimSpace(addOpts.KeyPath),
		AuthRef:      strings.TrimSpace(addOpts.AuthRef),
		Jump:         strings.TrimSpace(addOpts.Jump),
		Backend:      strings.TrimSpace(addOpts.Backend),
		Via:          strings.TrimSpace(addOpts.Via),
		RunTemplate:  strings.TrimSpace(addOpts.RunTemplate),
		LoginCommand: strings.TrimSpace(addOpts.LoginCommand),
		Remark:       strings.TrimSpace(addOpts.Remark),
		Group:        strings.TrimSpace(addOpts.Group),
		Tags:         core.NormalizeTags(addOpts.Tags),
		Port:         addOpts.Port,
	}
	normalizeHostDefaults(&host)
	return host, nil
}

func applyHostKeyOptions(host *core.Host, embedKey bool, passphraseFlag keyPassphraseFlag) error {
	if embedKey {
		keyData, err := readKeyFileContent(host.KeyPath)
		if err != nil {
			return err
		}
		host.KeyData = keyData
	}
	if passphraseFlag.SetFlag {
		if strings.TrimSpace(host.KeyPath) == "" && strings.TrimSpace(host.KeyData) == "" && strings.TrimSpace(host.KeyDataEnc) == "" {
			return fmt.Errorf("--key-passphrase requires --key")
		}
		passphrase, err := resolveKeyPassphrase(passphraseFlag)
		if err != nil {
			return err
		}
		host.KeyPassphrase = passphrase
	}
	return nil
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
	tags, err := promptLine(reader, output, "Tags", "")
	if err != nil {
		return host, err
	}
	host.Tags = core.NormalizeTags(tags)
	if host.Jump, err = promptLine(reader, output, "Jump", ""); err != nil {
		return host, err
	}
	if host.Backend, err = promptLine(reader, output, "Backend", ""); err != nil {
		return host, err
	}
	if host.Via, err = promptLine(reader, output, "Via", ""); err != nil {
		return host, err
	}
	if host.RunTemplate, err = promptLine(reader, output, "Run template", ""); err != nil {
		return host, err
	}
	if host.LoginCommand, err = promptLine(reader, output, "Login command", ""); err != nil {
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
	core.NormalizeHostFields(host)
	if host.Name == "" {
		host.Name = host.IP
	}
	if host.Group == "" {
		host.Group = core.DefaultGroup
	}
}
