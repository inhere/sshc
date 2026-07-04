package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/gcli/v3"
)

const managementCategory = "Management"

var runEditor = runConfigEditor

func NewCfgCmd() *gcli.Command {
	cmd := &gcli.Command{
		Name:     "cfg",
		Desc:     "manage sshc config",
		Aliases:  []string{"config"},
		Category: managementCategory,
		Help: strings.TrimSpace(`
Examples:
  sshc cfg path
  sshc cfg show
  sshc cfg show --raw
  sshc cfg get logs_path
  sshc cfg set logs_path ./runtime/logs
  sshc cfg unset logs_path
  sshc cfg doctor
  sshc cfg edit

Notes:
  - show masks passwords and encrypted password values by default.
  - show --raw prints the config file as stored on disk and may expose secrets.
  - get, set, and unset initially support logs_path only.
`),
	}
	cmd.Add(
		newCfgPathCmd(),
		newCfgShowCmd(),
		newCfgGetCmd(),
		newCfgSetCmd(),
		newCfgUnsetCmd(),
		newCfgEditCmd(),
		newCfgDoctorCmd(),
	)
	return cmd
}

func newCfgPathCmd() *gcli.Command {
	return &gcli.Command{
		Name: "path",
		Desc: "print config file path",
		Func: func(c *gcli.Command, _ []string) error {
			path, err := core.StorePath()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmdOutput(c), path)
			if strings.TrimSpace(os.Getenv(core.ConfigEnvKey)) != "" {
				fmt.Fprintf(cmdOutput(c), "source=%s\n", core.ConfigEnvKey)
			} else {
				fmt.Fprintln(cmdOutput(c), "source=default")
			}
			return nil
		},
	}
}

func newCfgShowCmd() *gcli.Command {
	var raw bool
	return &gcli.Command{
		Name: "show",
		Desc: "show config json",
		Config: func(c *gcli.Command) {
			c.BoolOpt(&raw, "raw", "", false, "show raw config file")
		},
		Func: func(c *gcli.Command, _ []string) error {
			if raw {
				_, data, err := core.ReadConfigFile()
				if err != nil {
					return err
				}
				if len(strings.TrimSpace(string(data))) == 0 {
					fmt.Fprintln(cmdOutput(c), "{}")
					return nil
				}
				fmt.Fprint(cmdOutput(c), string(data))
				if !strings.HasSuffix(string(data), "\n") {
					fmt.Fprintln(cmdOutput(c))
				}
				return nil
			}

			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			return writeJSON(c, core.MaskConfig(*config))
		},
	}
}

func newCfgGetCmd() *gcli.Command {
	return &gcli.Command{
		Name: "get",
		Desc: "get config value",
		Config: func(c *gcli.Command) {
			c.AddArg("key", "config key", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			key := strings.TrimSpace(c.Arg("key").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			switch key {
			case "logs_path":
				fmt.Fprintln(cmdOutput(c), config.LogsPath)
				return nil
			default:
				return unsupportedCfgKey(key)
			}
		},
	}
}

func newCfgSetCmd() *gcli.Command {
	return &gcli.Command{
		Name: "set",
		Desc: "set config value",
		Config: func(c *gcli.Command) {
			c.AddArg("key", "config key", true)
			c.AddArg("value", "config value", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			key := strings.TrimSpace(c.Arg("key").String())
			value := strings.TrimSpace(c.Arg("value").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			switch key {
			case "logs_path":
				config.LogsPath = value
			default:
				return unsupportedCfgKey(key)
			}
			if err := core.SaveConfig(config); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "set %s=%s\n", key, value)
			return nil
		},
	}
}

func newCfgUnsetCmd() *gcli.Command {
	return &gcli.Command{
		Name: "unset",
		Desc: "unset config value",
		Config: func(c *gcli.Command) {
			c.AddArg("key", "config key", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			key := strings.TrimSpace(c.Arg("key").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			switch key {
			case "logs_path":
				config.LogsPath = ""
			default:
				return unsupportedCfgKey(key)
			}
			if err := core.SaveConfig(config); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "unset %s\n", key)
			return nil
		},
	}
}

func newCfgEditCmd() *gcli.Command {
	return &gcli.Command{
		Name: "edit",
		Desc: "open config in editor",
		Func: func(c *gcli.Command, _ []string) error {
			path, err := core.StorePath()
			if err != nil {
				return err
			}
			if err := ensureConfigFile(path); err != nil {
				return err
			}
			editor := strings.TrimSpace(os.Getenv("VISUAL"))
			if editor == "" {
				editor = strings.TrimSpace(os.Getenv("EDITOR"))
			}
			if editor == "" {
				fmt.Fprintf(cmdOutput(c), "config path: %s\n", path)
				return errors.New("VISUAL or EDITOR is required")
			}
			return runEditor(editor, path)
		},
	}
}

func newCfgDoctorCmd() *gcli.Command {
	return &gcli.Command{
		Name: "doctor",
		Desc: "check local config",
		Func: func(c *gcli.Command, _ []string) error {
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			issues := core.CheckConfig(*config)
			for _, issue := range issues {
				fmt.Fprintf(cmdOutput(c), "%s\t%s\t%s\n", issue.Level, issue.Item, issue.Message)
			}
			if core.HasDoctorErrors(issues) {
				return errors.New("config doctor found errors")
			}
			return nil
		},
	}
}

func writeJSON(c *gcli.Command, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmdOutput(c), string(data))
	return nil
}

func unsupportedCfgKey(key string) error {
	return fmt.Errorf("unsupported config key %q, currently only logs_path is supported", key)
}

func ensureConfigFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return core.SaveConfig(&core.Config{})
}

func runConfigEditor(editor, path string) error {
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
