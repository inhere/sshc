package command

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gookit/cliui/show/table"
	"github.com/gookit/gcli/v3"
	"github.com/inhere/sshc/internal/core"
)

func NewGroupCmd() *gcli.Command {
	cmd := &gcli.Command{
		Name:     "group",
		Desc:     "manage group defaults",
		Aliases:  []string{"groups"},
		Category: managementCategory,
		Help: strings.TrimSpace(`
Examples:
  sshc group list
  sshc group show testing
  sshc group set testing auth=dev-root jump=bastion port=22
  sshc group unset testing jump port
  sshc group rm testing --yes

Notes:
  - Group defaults are inherited by hosts in the same group.
  - Host fields override group defaults.
`),
	}
	cmd.Add(
		newGroupListCmd(),
		newGroupShowCmd(),
		newGroupSetCmd(),
		newGroupUnsetCmd(),
		newGroupRemoveCmd(),
	)
	return cmd
}

func newGroupListCmd() *gcli.Command {
	var jsonOut bool
	return &gcli.Command{
		Name:    "list",
		Desc:    "list group defaults",
		Aliases: []string{"ls"},
		Config: func(c *gcli.Command) {
			c.BoolOpt(&jsonOut, "json", "", false, "output groups as json")
		},
		Func: func(c *gcli.Command, _ []string) error {
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			if jsonOut {
				return writeJSON(c, config.Groups)
			}
			out := buildGroupListTable(config.Groups)
			if out != "" {
				fmt.Fprint(cmdOutput(c), out)
			}
			return nil
		},
	}
}

func newGroupShowCmd() *gcli.Command {
	return &gcli.Command{
		Name: "show",
		Desc: "show group defaults",
		Config: func(c *gcli.Command) {
			c.AddArg("name", "group name", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			name := strings.TrimSpace(c.Arg("name").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			group, ok := config.Groups[name]
			if !ok {
				return fmt.Errorf("group %q not found", name)
			}
			return writeJSON(c, group)
		},
	}
}

func newGroupSetCmd() *gcli.Command {
	return &gcli.Command{
		Name: "set",
		Desc: "set group default fields",
		Config: func(c *gcli.Command) {
			c.AddArg("name", "group name", true)
			c.AddArg("fields", "group fields in key=value form", false, true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			name := strings.TrimSpace(c.Arg("name").String())
			fields := c.Arg("fields").Strings()
			if name == "" {
				return errors.New("group name is required")
			}
			if len(fields) == 0 {
				return errors.New("no group field provided")
			}
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			if config.Groups == nil {
				config.Groups = map[string]core.GroupDefaults{}
			}
			group := config.Groups[name]
			changed, err := setGroupFieldsFromArgs(&group, fields)
			if err != nil {
				return err
			}
			updated := *config
			updated.Groups = cloneGroups(config.Groups)
			updated.Groups[name] = group
			if issues := core.CheckConfig(updated); core.HasDoctorErrors(issues) {
				return errors.New(formatDoctorErrors(issues))
			}
			config.Groups = updated.Groups
			if err := core.SaveConfig(config); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "updated group %s (%d fields)\n", name, changed)
			return nil
		},
	}
}

func newGroupUnsetCmd() *gcli.Command {
	return &gcli.Command{
		Name: "unset",
		Desc: "unset group default fields",
		Config: func(c *gcli.Command) {
			c.AddArg("name", "group name", true)
			c.AddArg("fields", "group field names to unset", false, true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			name := strings.TrimSpace(c.Arg("name").String())
			fields := c.Arg("fields").Strings()
			if len(fields) == 0 {
				return errors.New("no group field provided")
			}
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			group, ok := config.Groups[name]
			if !ok {
				return fmt.Errorf("group %q not found", name)
			}
			changed, err := unsetGroupFieldsFromArgs(&group, fields)
			if err != nil {
				return err
			}
			updated := *config
			updated.Groups = cloneGroups(config.Groups)
			updated.Groups[name] = group
			if issues := core.CheckConfig(updated); core.HasDoctorErrors(issues) {
				return errors.New(formatDoctorErrors(issues))
			}
			config.Groups = updated.Groups
			if err := core.SaveConfig(config); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "updated group %s (%d fields)\n", name, changed)
			return nil
		},
	}
}

func newGroupRemoveCmd() *gcli.Command {
	var yes bool
	return &gcli.Command{
		Name:    "rm",
		Desc:    "remove group defaults",
		Aliases: []string{"remove", "delete"},
		Config: func(c *gcli.Command) {
			c.BoolOpt(&yes, "yes", "y", false, "confirm removal")
			c.AddArg("name", "group name", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			name := strings.TrimSpace(c.Arg("name").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			if _, ok := config.Groups[name]; !ok {
				return fmt.Errorf("group %q not found", name)
			}
			if !yes {
				if ok, err := confirmInteractive(fmt.Sprintf("remove group %s?", name)); err != nil {
					return err
				} else if !ok {
					return errors.New("remove canceled")
				}
			}
			delete(config.Groups, name)
			if err := core.SaveConfig(config); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "removed group %s\n", name)
			return nil
		},
	}
}

func buildGroupListTable(groups map[string]core.GroupDefaults) string {
	if len(groups) == 0 {
		return ""
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	tb := table.New("", table.WithBorderFlags(table.BorderDefault), table.WithOverflowFlag(table.OverflowWrap))
	tb.SetHeads("Name", "Auth", "User", "Port", "Jump", "Timeouts")
	for _, name := range names {
		group := groups[name]
		tb.AddRow(name, dash(group.AuthRef), dash(group.User), groupPortLabel(group.Port), dash(group.Jump), groupTimeoutLabel(group))
	}
	return tb.String()
}

func setGroupFieldsFromArgs(group *core.GroupDefaults, fields []string) (int, error) {
	changed := 0
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return 0, fmt.Errorf("invalid group field %q, expected key=value", field)
		}
		key = normalizeGroupField(key)
		if err := setGroupField(group, key, strings.TrimSpace(value)); err != nil {
			return 0, err
		}
		changed++
	}
	core.NormalizeGroupDefaults(group)
	return changed, nil
}

func setGroupField(group *core.GroupDefaults, key, value string) error {
	switch key {
	case "auth_ref":
		group.AuthRef = value
	case "user":
		group.User = value
	case "key_path":
		group.KeyPath = value
	case "port":
		port, err := parseGroupPort(value)
		if err != nil {
			return err
		}
		group.Port = port
	case "jump":
		group.Jump = value
	case "connect_timeout":
		group.ConnectTimeout = value
	case "run_timeout":
		group.RunTimeout = value
	case "remote_script_dir":
		group.RemoteScriptDir = value
	case "host_key_check":
		group.HostKeyCheck = value
	case "known_hosts_path":
		group.KnownHostsPath = value
	default:
		return fmt.Errorf("unknown group field %q", key)
	}
	return nil
}

func unsetGroupFieldsFromArgs(group *core.GroupDefaults, fields []string) (int, error) {
	changed := 0
	for _, field := range fields {
		key := normalizeGroupField(field)
		if err := unsetGroupField(group, key); err != nil {
			return 0, err
		}
		changed++
	}
	core.NormalizeGroupDefaults(group)
	return changed, nil
}

func unsetGroupField(group *core.GroupDefaults, key string) error {
	switch key {
	case "auth_ref":
		group.AuthRef = ""
	case "user":
		group.User = ""
	case "key_path":
		group.KeyPath = ""
	case "port":
		group.Port = 0
	case "jump":
		group.Jump = ""
	case "connect_timeout":
		group.ConnectTimeout = ""
	case "run_timeout":
		group.RunTimeout = ""
	case "remote_script_dir":
		group.RemoteScriptDir = ""
	case "host_key_check":
		group.HostKeyCheck = ""
	case "known_hosts_path":
		group.KnownHostsPath = ""
	default:
		return fmt.Errorf("unknown group field %q", key)
	}
	return nil
}

func normalizeGroupField(field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	field = strings.ReplaceAll(field, "-", "_")
	switch field {
	case "auth":
		return "auth_ref"
	case "key", "keypath", "keyfile":
		return "key_path"
	default:
		return field
	}
}

func parseGroupPort(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("port cannot be empty")
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid group port %q", value)
	}
	return port, nil
}

func cloneGroups(groups map[string]core.GroupDefaults) map[string]core.GroupDefaults {
	cloned := make(map[string]core.GroupDefaults, len(groups))
	for name, group := range groups {
		cloned[name] = group
	}
	return cloned
}

func dash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return strings.TrimSpace(value)
}

func groupPortLabel(port int) string {
	if port == 0 {
		return "-"
	}
	return strconv.Itoa(port)
}

func groupTimeoutLabel(group core.GroupDefaults) string {
	items := []string{}
	if strings.TrimSpace(group.ConnectTimeout) != "" {
		items = append(items, "connect="+strings.TrimSpace(group.ConnectTimeout))
	}
	if strings.TrimSpace(group.RunTimeout) != "" {
		items = append(items, "run="+strings.TrimSpace(group.RunTimeout))
	}
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ",")
}
