package command

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/cliui/show/table"
	"github.com/gookit/gcli/v3"
	"golang.org/x/term"
)

func NewAuthCmd() *gcli.Command {
	cmd := &gcli.Command{
		Name:     "auth",
		Desc:     "manage credential profiles",
		Aliases:  []string{"cred", "creds"},
		Category: managementCategory,
		Help: strings.TrimSpace(`
Examples:
  sshc auth add dev-root -u root -p --remark "shared root login"
  sshc auth add deploy-key -u deploy --key ~/.ssh/id_ed25519
  sshc auth add deploy-key -u deploy --key ~/.ssh/id_ed25519 --embed-key
  sshc auth add deploy-key -u deploy --key ~/.ssh/id_ed25519 --embed-key --key-passphrase
  sshc auth add deploy-key -u deploy --key ~/.ssh/id_ed25519 --key-passphrase=env
  sshc auth show dev-root
  sshc auth show dev-root --raw
  sshc auth show dev-root --reveal
  sshc auth rm dev-root --yes

Notes:
  - -p/--password prompts for a hidden password and does not accept an inline value.
  - --embed-key stores encrypted private key content in sshc.config.json.
  - --key-passphrase reads a private key passphrase from hidden input by default.
  - --key-passphrase=clip reads it from clipboard; --key-passphrase=env reads ${SSHC_KEY_PASSPHRASE}.
  - Passwords are encrypted before writing sshc.config.json.
  - auth show masks secrets by default; --raw prints the stored value
    (encrypted when it was saved encrypted), --reveal prints the decrypted
    secret, so keep its output out of logs and screenshots.
  - auth rm refuses profiles still referenced by any host.
`),
	}
	cmd.Add(
		newAuthAddCmd(),
		newAuthListCmd(),
		newAuthShowCmd(),
		newAuthRemoveCmd(),
	)
	return cmd
}

func newAuthAddCmd() *gcli.Command {
	opts := struct {
		User             string
		PasswordPrompt   bool
		PasswordCmd      string
		KeyPath          string
		EmbedKey         bool
		KeyPassphrase    keyPassphraseFlag
		KeyPassphraseCmd string
		Remark           string
	}{}
	return &gcli.Command{
		Name: "add",
		Desc: "add or update credential profile",
		Config: func(c *gcli.Command) {
			c.StrOpt(&opts.User, "user", "u", "", "ssh username")
			c.BoolOpt(&opts.PasswordPrompt, "password", "p", false, "prompt for hidden password")
			c.StrOpt(&opts.PasswordCmd, "password-cmd", "", "", "command whose output is the password")
			c.StrOpt(&opts.KeyPath, "key", "", "", "ssh private key path")
			c.BoolOpt(&opts.EmbedKey, "embed-key", "", false, "store encrypted private key content in config")
			c.VarOpt(&opts.KeyPassphrase, "key-passphrase", "", "key passphrase source: input, clip, or env")
			c.StrOpt(&opts.KeyPassphraseCmd, "key-passphrase-cmd", "", "", "command whose output is the key passphrase")
			c.StrOpt(&opts.Remark, "remark", "", "", "auth profile remark")
			c.AddArg("name", "auth profile name", true)
		},
		Func: func(c *gcli.Command, args []string) error {
			name := strings.TrimSpace(c.Arg("name").String())
			var err error
			name, args, err = consumeAuthAddKeyPassphraseSourceArg(&opts.KeyPassphrase, name, args)
			if err != nil {
				return err
			}
			if len(args) > 0 {
				return errors.New("-p/--password does not accept an inline value")
			}
			profile := core.AuthProfile{
				Name:        name,
				User:        strings.TrimSpace(opts.User),
				PasswordCmd: strings.TrimSpace(opts.PasswordCmd),
				Remark:      strings.TrimSpace(opts.Remark),
			}
			if opts.PasswordPrompt && profile.PasswordCmd != "" {
				return errors.New("-p/--password and --password-cmd cannot be used together")
			}
			if opts.KeyPassphrase.SetFlag && strings.TrimSpace(opts.KeyPassphraseCmd) != "" {
				return errors.New("--key-passphrase and --key-passphrase-cmd cannot be used together")
			}
			profile.KeyPassphraseCmd = strings.TrimSpace(opts.KeyPassphraseCmd)
			keyPath, err := normalizeKeyPathForSave(opts.KeyPath)
			if err != nil {
				return err
			}
			profile.KeyPath = keyPath
			if err := applyAuthProfileKeyOptions(&profile, opts.EmbedKey, opts.KeyPassphrase); err != nil {
				return err
			}
			if opts.PasswordPrompt {
				profile.Password = strings.TrimSpace(readInteractivePassword("Password: "))
			}
			if err := validateAuthProfile(profile); err != nil {
				return err
			}

			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			upsertAuthProfile(config, profile)
			if err := core.SaveConfig(config); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "saved auth profile %s\n", profile.Name)
			return nil
		},
	}
}

func newAuthListCmd() *gcli.Command {
	return &gcli.Command{
		Name:    "list",
		Desc:    "list credential profiles",
		Aliases: []string{"ls"},
		Func: func(c *gcli.Command, _ []string) error {
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			out := buildAuthListTable(config.AuthProfiles)
			if out != "" {
				fmt.Fprint(cmdOutput(c), out)
			}
			return nil
		},
	}
}

func newAuthShowCmd() *gcli.Command {
	var raw bool
	var reveal bool
	return &gcli.Command{
		Name: "show",
		Desc: "show credential profile",
		Config: func(c *gcli.Command) {
			c.BoolOpt(&raw, "raw", "", false, "show raw profile from config file")
			c.BoolOpt(&reveal, "reveal", "", false, "show decrypted secret values")
			c.AddArg("name", "auth profile name", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			name := strings.TrimSpace(c.Arg("name").String())
			if raw && reveal {
				return errors.New("--raw and --reveal cannot be used together")
			}
			if raw {
				profile, ok, err := loadRawAuthProfile(name)
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("auth profile %q not found", name)
				}
				return writeJSON(c, profile)
			}

			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			profile, ok := config.FindAuthProfile(name)
			if !ok {
				return fmt.Errorf("auth profile %q not found", name)
			}
			if reveal {
				// LoadConfig already decrypted the stored secrets of this profile.
				return writeJSON(c, profile)
			}
			return writeJSON(c, core.MaskAuthProfile(profile))
		},
	}
}

func newAuthRemoveCmd() *gcli.Command {
	var yes bool
	return &gcli.Command{
		Name:    "rm",
		Desc:    "remove credential profile",
		Aliases: []string{"remove", "delete"},
		Config: func(c *gcli.Command) {
			c.BoolOpt(&yes, "yes", "y", false, "confirm removal")
			c.AddArg("name", "auth profile name", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			name := strings.TrimSpace(c.Arg("name").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			if _, ok := config.FindAuthProfile(name); !ok {
				return fmt.Errorf("auth profile %q not found", name)
			}
			if used := hostsUsingAuth(config.Hosts, name); len(used) > 0 {
				return fmt.Errorf("auth profile %q is used by host(s): %s", name, strings.Join(used, ", "))
			}
			if used := core.TunnelsUsingAuth(config.Tunnels, name); len(used) > 0 {
				return fmt.Errorf("auth profile %q is used by tunnel(s): %s", name, strings.Join(used, ", "))
			}
			if !yes {
				if ok, err := confirmInteractive(fmt.Sprintf("remove auth profile %s?", name)); err != nil {
					return err
				} else if !ok {
					return errors.New("remove canceled")
				}
			}
			config.AuthProfiles = removeAuthProfile(config.AuthProfiles, name)
			if err := core.SaveConfig(config); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "removed auth profile %s\n", name)
			return nil
		},
	}
}

func consumeAuthAddKeyPassphraseSourceArg(flag *keyPassphraseFlag, name string, args []string) (string, []string, error) {
	if flag == nil || !flag.SetFlag || flag.Source != "input" {
		return name, args, nil
	}
	remaining, err := consumeKeyPassphraseSourceArg(flag, args)
	return name, remaining, err
}

func validateAuthProfile(profile core.AuthProfile) error {
	if strings.TrimSpace(profile.Name) == "" {
		return errors.New("auth profile name is required")
	}
	if !authProfileHasPassword(profile) && !authProfileHasKey(profile) {
		return errors.New("password or key is required")
	}
	if strings.TrimSpace(profile.PasswordCmd) != "" && (profile.Password != "" || profile.PasswordEnc != "") {
		return errors.New("password and password_cmd cannot both be set")
	}
	if strings.TrimSpace(profile.KeyPassphraseCmd) != "" {
		if !authProfileHasKey(profile) {
			return errors.New("--key-passphrase-cmd requires --key")
		}
		if profile.KeyPassphrase != "" || profile.KeyPassphraseEnc != "" {
			return errors.New("key_passphrase and key_passphrase_cmd cannot both be set")
		}
	}
	return nil
}

func applyAuthProfileKeyOptions(profile *core.AuthProfile, embedKey bool, passphraseFlag keyPassphraseFlag) error {
	if embedKey {
		keyData, err := readKeyFileContent(profile.KeyPath)
		if err != nil {
			return err
		}
		profile.KeyData = keyData
	}
	if passphraseFlag.SetFlag {
		if strings.TrimSpace(profile.KeyPath) == "" && strings.TrimSpace(profile.KeyData) == "" && strings.TrimSpace(profile.KeyDataEnc) == "" {
			return fmt.Errorf("--key-passphrase requires --key")
		}
		passphrase, err := resolveKeyPassphrase(passphraseFlag)
		if err != nil {
			return err
		}
		profile.KeyPassphrase = passphrase
	}
	return nil
}

func upsertAuthProfile(config *core.Config, profile core.AuthProfile) {
	for i, item := range config.AuthProfiles {
		if strings.TrimSpace(item.Name) == profile.Name {
			config.AuthProfiles[i] = profile
			return
		}
	}
	config.AuthProfiles = append(config.AuthProfiles, profile)
}

func buildAuthListTable(profiles []core.AuthProfile) string {
	if len(profiles) == 0 {
		return ""
	}
	tb := table.New("", table.WithBorderFlags(table.BorderDefault), table.WithOverflowFlag(table.OverflowWrap))
	tb.SetHeads("Name", "User", "Auth", "Remark")
	for _, profile := range profiles {
		user := strings.TrimSpace(profile.User)
		if user == "" {
			user = "-"
		}
		remark := strings.TrimSpace(profile.Remark)
		if remark == "" {
			remark = "-"
		}
		tb.AddRow(profile.Name, user, authProfileType(profile), remark)
	}
	return tb.String()
}

func authProfileType(profile core.AuthProfile) string {
	hasKey := authProfileHasKey(profile)
	hasPassword := authProfileHasPassword(profile)
	switch {
	case hasKey && hasPassword:
		return "key+password"
	case hasKey:
		return "key"
	case hasPassword:
		return "password"
	default:
		return "-"
	}
}

func authProfileHasKey(profile core.AuthProfile) bool {
	return strings.TrimSpace(profile.KeyPath) != "" ||
		strings.TrimSpace(profile.KeyData) != "" ||
		strings.TrimSpace(profile.KeyDataEnc) != ""
}

func authProfileHasPassword(profile core.AuthProfile) bool {
	return profile.Password != "" || profile.PasswordEnc != "" || strings.TrimSpace(profile.PasswordCmd) != ""
}

func loadRawAuthProfile(name string) (core.AuthProfile, bool, error) {
	_, data, err := core.ReadConfigFile()
	if err != nil {
		return core.AuthProfile{}, false, err
	}
	var config core.Config
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &config); err != nil {
			return core.AuthProfile{}, false, err
		}
	}
	profile, ok := config.FindAuthProfile(name)
	return profile, ok, nil
}

func hostsUsingAuth(hosts []core.Host, name string) []string {
	var used []string
	for _, host := range hosts {
		if strings.TrimSpace(host.AuthRef) == name {
			used = append(used, core.HostLogName(host))
		}
	}
	return used
}

func removeAuthProfile(profiles []core.AuthProfile, name string) []core.AuthProfile {
	filtered := profiles[:0]
	for _, profile := range profiles {
		if strings.TrimSpace(profile.Name) != name {
			filtered = append(filtered, profile)
		}
	}
	return filtered
}

func confirmInteractive(question string) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, errors.New("confirmation required; use --yes in non-interactive mode")
	}
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", question)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
