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
  sshc auth show dev-root
  sshc auth show dev-root --raw
  sshc auth rm dev-root --yes

Notes:
  - -p/--password prompts for a hidden password and does not accept an inline value.
  - Passwords are encrypted before writing sshc.config.json.
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
		User           string
		PasswordPrompt bool
		KeyPath        string
		Remark         string
	}{}
	return &gcli.Command{
		Name: "add",
		Desc: "add or update credential profile",
		Config: func(c *gcli.Command) {
			c.StrOpt(&opts.User, "user", "u", "", "ssh username")
			c.BoolOpt(&opts.PasswordPrompt, "password", "p", false, "prompt for hidden password")
			c.StrOpt(&opts.KeyPath, "key", "", "", "ssh private key path")
			c.StrOpt(&opts.Remark, "remark", "", "", "auth profile remark")
			c.AddArg("name", "auth profile name", true)
		},
		Func: func(c *gcli.Command, args []string) error {
			if len(args) > 0 {
				return errors.New("-p/--password does not accept an inline value")
			}
			name := strings.TrimSpace(c.Arg("name").String())
			profile := core.AuthProfile{
				Name:   name,
				User:   strings.TrimSpace(opts.User),
				Remark: strings.TrimSpace(opts.Remark),
			}
			keyPath, err := normalizeKeyPathForSave(opts.KeyPath)
			if err != nil {
				return err
			}
			profile.KeyPath = keyPath
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
	return &gcli.Command{
		Name: "show",
		Desc: "show credential profile",
		Config: func(c *gcli.Command) {
			c.BoolOpt(&raw, "raw", "", false, "show raw profile from config file")
			c.AddArg("name", "auth profile name", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			name := strings.TrimSpace(c.Arg("name").String())
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

func validateAuthProfile(profile core.AuthProfile) error {
	if strings.TrimSpace(profile.Name) == "" {
		return errors.New("auth profile name is required")
	}
	if profile.Password == "" && strings.TrimSpace(profile.KeyPath) == "" {
		return errors.New("password or key is required")
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
	hasKey := strings.TrimSpace(profile.KeyPath) != ""
	hasPassword := profile.Password != "" || profile.PasswordEnc != ""
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
