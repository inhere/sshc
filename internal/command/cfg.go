package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
  sshc cfg show --raw
  sshc cfg get logs_path
  sshc cfg set logs_path ./runtime/logs
  sshc cfg set defaults.user root
  sshc cfg set defaults.port 2222
  sshc cfg set defaults.host_key_check known_hosts
  sshc cfg unset logs_path
  sshc cfg export -o sshc-export.enc
  sshc cfg import -f sshc-export.enc --key "sshc-v1:..."

Notes:
  - show masks passwords and encrypted password values by default.
  - show --raw prints the config file as stored on disk and may expose secrets.
  - get, set, and unset only support whitelisted config keys.
  - import uses merge by default; use --overwrite or --replace explicitly to change existing entries.
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
		newCfgExportCmd(),
		newCfgImportCmd(),
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
			value, err := getConfigValue(config, key)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmdOutput(c), value)
			return nil
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
			if err := setConfigValue(config, key, value); err != nil {
				return err
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
			if err := unsetConfigValue(config, key); err != nil {
				return err
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

func newCfgExportCmd() *gcli.Command {
	opts := struct {
		Output string
		Force  bool
	}{}
	return &gcli.Command{
		Name: "export",
		Desc: "export encrypted config",
		Config: func(c *gcli.Command) {
			c.StrOpt(&opts.Output, "output", "o", "", "output export file")
			c.BoolOpt(&opts.Force, "force", "", false, "overwrite existing output file")
		},
		Func: func(c *gcli.Command, _ []string) error {
			output := strings.TrimSpace(opts.Output)
			if output == "" {
				return errors.New("--output is required")
			}
			if !opts.Force {
				if _, err := os.Stat(output); err == nil {
					return fmt.Errorf("output file %s already exists; use --force to overwrite", output)
				} else if !os.IsNotExist(err) {
					return err
				}
			}
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			key, err := core.GenerateExportKey()
			if err != nil {
				return err
			}
			data, err := core.EncryptConfigExport(*config, key, time.Now())
			if err != nil {
				return err
			}
			if dir := filepath.Dir(output); dir != "." {
				if err := os.MkdirAll(dir, 0700); err != nil {
					return err
				}
			}
			if err := os.WriteFile(output, data, 0600); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "exported config to %s\n", output)
			fmt.Fprintf(cmdOutput(c), "export key: %s\n", key)
			return nil
		},
	}
}

func newCfgImportCmd() *gcli.Command {
	opts := struct {
		File      string
		Key       string
		Merge     bool
		Overwrite bool
		Replace   bool
	}{}
	return &gcli.Command{
		Name: "import",
		Desc: "import encrypted config",
		Config: func(c *gcli.Command) {
			c.StrOpt(&opts.File, "file", "f", "", "input export file")
			c.StrOpt(&opts.Key, "key", "", "", "export key")
			c.BoolOpt(&opts.Merge, "merge", "", false, "merge imported config")
			c.BoolOpt(&opts.Overwrite, "overwrite", "", false, "overwrite conflicting entries")
			c.BoolOpt(&opts.Replace, "replace", "", false, "replace current config")
		},
		Func: func(c *gcli.Command, _ []string) error {
			file := strings.TrimSpace(opts.File)
			if file == "" {
				return errors.New("--file is required")
			}
			key := strings.TrimSpace(opts.Key)
			if key == "" {
				return errors.New("--key is required")
			}
			strategy, err := cfgImportStrategy(opts.Merge, opts.Overwrite, opts.Replace)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			imported, err := core.DecryptConfigExport(data, key)
			if err != nil {
				return err
			}
			current, err := core.LoadConfig()
			if err != nil {
				return err
			}
			merged, result, err := core.MergeImportedConfig(*current, imported, strategy)
			if err != nil {
				return err
			}
			backupPath, err := core.BackupConfigFile(time.Now())
			if err != nil {
				return err
			}
			result.BackupPath = backupPath
			if err := core.SaveConfig(&merged); err != nil {
				return err
			}
			if result.BackupPath == "" {
				fmt.Fprintln(cmdOutput(c), "backup: none")
			} else {
				fmt.Fprintf(cmdOutput(c), "backup: %s\n", result.BackupPath)
			}
			fmt.Fprintf(cmdOutput(c), "imported config: hosts_added=%d hosts_updated=%d auth_added=%d auth_updated=%d\n", result.HostsAdded, result.HostsUpdated, result.AuthAdded, result.AuthUpdated)
			return nil
		},
	}
}

func cfgImportStrategy(merge, overwrite, replace bool) (core.ImportStrategy, error) {
	count := 0
	for _, enabled := range []bool{merge, overwrite, replace} {
		if enabled {
			count++
		}
	}
	if count > 1 {
		return "", errors.New("--merge, --overwrite and --replace cannot be used together")
	}
	switch {
	case overwrite:
		return core.ImportOverwrite, nil
	case replace:
		return core.ImportReplace, nil
	default:
		return core.ImportMerge, nil
	}
}

func getConfigValue(config *core.Config, key string) (string, error) {
	switch key {
	case "logs_path":
		return config.LogsPath, nil
	case "defaults.user":
		return config.Defaults.User, nil
	case "defaults.port":
		if config.Defaults.Port == 0 {
			return "", nil
		}
		return strconv.Itoa(config.Defaults.Port), nil
	case "defaults.connect_timeout":
		return config.Defaults.ConnectTimeout, nil
	case "defaults.run_timeout":
		return config.Defaults.RunTimeout, nil
	case "defaults.remote_script_dir":
		return config.Defaults.RemoteScriptDir, nil
	case "defaults.host_key_check":
		return config.Defaults.HostKeyCheck, nil
	case "defaults.known_hosts_path":
		return config.Defaults.KnownHostsPath, nil
	default:
		return "", unsupportedCfgKey(key)
	}
}

func setConfigValue(config *core.Config, key, value string) error {
	switch key {
	case "logs_path":
		config.LogsPath = value
	case "defaults.user":
		config.Defaults.User = value
	case "defaults.port":
		port, err := parseCfgPort(value)
		if err != nil {
			return err
		}
		config.Defaults.Port = port
	case "defaults.connect_timeout":
		config.Defaults.ConnectTimeout = value
	case "defaults.run_timeout":
		config.Defaults.RunTimeout = value
	case "defaults.remote_script_dir":
		config.Defaults.RemoteScriptDir = value
	case "defaults.host_key_check":
		if err := validateCfgHostKeyCheck(value); err != nil {
			return err
		}
		config.Defaults.HostKeyCheck = value
	case "defaults.known_hosts_path":
		config.Defaults.KnownHostsPath = value
	default:
		return unsupportedCfgKey(key)
	}
	return nil
}

func unsetConfigValue(config *core.Config, key string) error {
	switch key {
	case "logs_path":
		config.LogsPath = ""
	case "defaults.user":
		config.Defaults.User = ""
	case "defaults.port":
		config.Defaults.Port = 0
	case "defaults.connect_timeout":
		config.Defaults.ConnectTimeout = ""
	case "defaults.run_timeout":
		config.Defaults.RunTimeout = ""
	case "defaults.remote_script_dir":
		config.Defaults.RemoteScriptDir = ""
	case "defaults.host_key_check":
		config.Defaults.HostKeyCheck = ""
	case "defaults.known_hosts_path":
		config.Defaults.KnownHostsPath = ""
	default:
		return unsupportedCfgKey(key)
	}
	return nil
}

func parseCfgPort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid defaults.port %q, want 1-65535", value)
	}
	return port, nil
}

func validateCfgHostKeyCheck(value string) error {
	switch value {
	case core.HostKeyCheckKnownHosts, core.HostKeyCheckInsecure:
		return nil
	default:
		return fmt.Errorf("invalid defaults.host_key_check %q, want known_hosts or insecure", value)
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
	return fmt.Errorf("unsupported config key %q", key)
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
