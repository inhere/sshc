package command

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gookit/cliui/cutypes"
	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/gcli/v3"
)

type hostImportOptions struct {
	File           string
	FromClipboard  bool
	Format         string
	AuthRef        string
	User           string
	KeyPath        string
	Group          string
	Remark         string
	Jump           string
	Backend        string
	Via            string
	RunTemplate    string
	LoginCommand   string
	HostKeyCheck   string
	KnownHostsPath string
	Port           int
	DryRun         bool
	SkipExisting   bool
	Overwrite      bool
	Yes            bool
}

type hostSetOptions struct {
	IP              string
	User            string
	AuthRef         string
	KeyPath         string
	Remark          string
	Group           string
	Jump            string
	Backend         string
	Via             string
	RunTemplate     string
	LoginCommand    string
	Port            int
	ConnectTimeout  string
	RunTimeout      string
	RemoteScriptDir string
	HostKeyCheck    string
	KnownHostsPath  string
}

type hostUnsetOptions struct {
	User            bool
	AuthRef         bool
	KeyPath         bool
	Remark          bool
	Group           bool
	Jump            bool
	Backend         bool
	Via             bool
	RunTemplate     bool
	LoginCommand    bool
	ConnectTimeout  bool
	RunTimeout      bool
	RemoteScriptDir bool
	HostKeyCheck    bool
	KnownHostsPath  bool
}

func NewHostCmd() *gcli.Command {
	cmd := &gcli.Command{
		Name:     "host",
		Desc:     "manage ssh hosts",
		Aliases:  []string{"hosts", "h"},
		Category: managementCategory,
		Help: strings.TrimSpace(`
Examples:
  sshc host add --ip 192.168.1.10 --name devhost --auth dev-root
  sshc host add --ip 10.0.0.8 --name inner-db --auth dev-root --jump bastion
  sshc host add --name lxc-app --backend command_proxy --via pve-host --run-template "pct exec 101 -- sh -lc {{cmd}}" --login-command "pct enter 101"
  sshc host list --group testing
  sshc host list --match devhost
  sshc host list --show-ip
  sshc host show devhost
  sshc host set lxc-app --run-template "pct exec 101 -- sh -lc {{cmd}}"
  sshc host set devhost --remark "testing host" --jump bastion
  sshc host unset devhost --remark --jump
  sshc host rm devhost --yes
  sshc host rename old-name new-name

Notes:
  - host add supports the same fields as top-level add and adds --auth.
  - host set/unset only change the selected host record.
  - command_proxy hosts are logical targets that execute through a configured via host.
  - host list masks IPv4 addresses unless --show-ip is set.
  - host show masks password fields unless --raw is set.
`),
	}
	cmd.Add(
		newHostAddCmd(),
		newHostImportCmd(),
		newHostListCmd(),
		newHostShowCmd(),
		newHostSetCmd(),
		newHostUnsetCmd(),
		newHostRemoveCmd(),
		newHostRenameCmd(),
	)
	return cmd
}

func newHostImportCmd() *gcli.Command {
	opts := hostImportOptions{}
	return &gcli.Command{
		Name: "import",
		Desc: "import ssh hosts",
		Help: strings.TrimSpace(`
Examples:
  sshc host import -f ips.txt --format ips --auth dev-root --group testing
  sshc host import -f hosts.txt --format plain --dry-run
  sshc host import -f hosts.csv --format csv --overwrite --yes
  sshc host import --from-clipboard --format plain --auth dev-root
`),
		Config: func(c *gcli.Command) {
			c.StrOpt(&opts.File, "file", "f", "", "input file, or - for stdin")
			c.BoolOpt(&opts.FromClipboard, "from-clipboard", "fc", false, "read import data from clipboard")
			c.StrOpt(&opts.Format, "format", "", "", "input format: ips, plain, or csv")
			c.StrOpt(&opts.AuthRef, "auth", "", "", "default auth profile name")
			c.StrOpt(&opts.User, "user", "u", "", "default ssh username")
			c.StrOpt(&opts.KeyPath, "key", "", "", "default ssh private key path")
			c.StrOpt(&opts.Group, "group", "", "", "default host group")
			c.StrOpt(&opts.Remark, "remark", "", "", "default host remark")
			c.StrOpt(&opts.Jump, "jump", "", "", "default jump host name or ip")
			c.StrOpt(&opts.Backend, "backend", "", "", "default host backend: ssh or command_proxy")
			c.StrOpt(&opts.Via, "via", "", "", "default command_proxy via host")
			c.StrOpt(&opts.RunTemplate, "run-template", "", "", "default command_proxy run template")
			c.StrOpt(&opts.LoginCommand, "login-command", "", "", "default command_proxy login command")
			c.IntOpt(&opts.Port, "port", "", 0, "default ssh port")
			c.StrOpt(&opts.HostKeyCheck, "host-key-check", "", "", "default host key check policy: known_hosts or insecure")
			c.StrOpt(&opts.KnownHostsPath, "known-hosts-path", "", "", "default known_hosts file path")
			c.BoolOpt(&opts.DryRun, "dry-run", "", false, "preview import plan without saving")
			c.BoolOpt(&opts.SkipExisting, "skip-existing", "", false, "skip existing hosts")
			c.BoolOpt(&opts.Overwrite, "overwrite", "", false, "overwrite existing hosts")
			c.BoolOpt(&opts.Yes, "yes", "y", false, "confirm import")
		},
		Func: func(c *gcli.Command, _ []string) error {
			data, source, err := readHostImportInput(opts)
			if err != nil {
				return err
			}
			format, err := resolveHostImportFormat(opts.Format, source, data)
			if err != nil {
				return err
			}
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			defaults := hostImportDefaults(opts, config.Defaults)
			hosts, parseErrs := core.ParseHostImport(bytes.NewReader(data), format, defaults)
			if len(parseErrs) > 0 {
				writeHostImportErrors(c, "invalid", parseErrs)
				return fmt.Errorf("host import parse failed: %d error(s)", len(parseErrs))
			}
			plan, planErr := core.PlanHostImport(*config, hosts, core.HostImportOptions{
				Format:       format,
				Defaults:     defaults,
				Overwrite:    opts.Overwrite,
				SkipExisting: opts.SkipExisting,
			})
			writeHostImportPlan(c, plan, opts.DryRun)
			if planErr != nil {
				if opts.DryRun {
					return nil
				}
				return planErr
			}
			if opts.DryRun {
				return nil
			}
			if !opts.Yes && plan.Added+plan.Updated+plan.Skipped > 1 {
				if ok, err := confirmInteractive("import hosts?"); err != nil {
					return err
				} else if !ok {
					return errors.New("import canceled")
				}
			}
			if err := core.ApplyHostImport(config, plan); err != nil {
				return err
			}
			if err := core.SaveConfig(config); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "Imported hosts: added=%d updated=%d skipped=%d\n", plan.Added, plan.Updated, plan.Skipped)
			return nil
		},
	}
}

func newHostAddCmd() *gcli.Command {
	cmd := NewAddCmd()
	cmd.Name = "add"
	cmd.Aliases = nil
	cmd.Desc = "add or update ssh host"
	return cmd
}

func readHostImportInput(opts hostImportOptions) ([]byte, string, error) {
	if strings.TrimSpace(opts.File) != "" && opts.FromClipboard {
		return nil, "", errors.New("--file and --from-clipboard cannot be used together")
	}
	if opts.FromClipboard {
		text, err := readClipboard()
		if err != nil {
			return nil, "", err
		}
		return []byte(text), "clipboard", nil
	}
	file := strings.TrimSpace(opts.File)
	switch file {
	case "":
		return nil, "", errors.New("--file or --from-clipboard is required")
	case "-":
		if cutypes.Input == nil {
			return nil, "", errors.New("stdin is not available")
		}
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(cutypes.Input); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "stdin", nil
	default:
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, "", err
		}
		return data, file, nil
	}
}

func resolveHostImportFormat(value, source string, data []byte) (core.HostImportFormat, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
	case string(core.HostImportIPs):
		return core.HostImportIPs, nil
	case string(core.HostImportPlain):
		return core.HostImportPlain, nil
	case string(core.HostImportCSV):
		return core.HostImportCSV, nil
	default:
		return "", fmt.Errorf("unsupported import format %q", value)
	}
	switch strings.ToLower(filepath.Ext(source)) {
	case ".csv":
		return core.HostImportCSV, nil
	case ".ips":
		return core.HostImportIPs, nil
	}
	if hostImportLooksPlain(data) {
		return core.HostImportPlain, nil
	}
	return core.HostImportIPs, nil
}

func hostImportLooksPlain(data []byte) bool {
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, _, ok := strings.Cut(line, "="); ok {
			return true
		}
		if key, _, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(key) != "" {
			return true
		}
	}
	return false
}

func hostImportDefaults(opts hostImportOptions, configDefaults core.Defaults) core.HostImportDefaults {
	group := strings.TrimSpace(opts.Group)
	if group == "" {
		group = core.DefaultGroup
	}
	port := opts.Port
	if port == 0 {
		port = configDefaults.Port
	}
	if port == 0 {
		port = core.DefaultSSHPort
	}
	hostKeyCheck := strings.TrimSpace(opts.HostKeyCheck)
	if hostKeyCheck == "" {
		hostKeyCheck = configDefaults.HostKeyCheck
	}
	knownHostsPath := strings.TrimSpace(opts.KnownHostsPath)
	if knownHostsPath == "" {
		knownHostsPath = configDefaults.KnownHostsPath
	}
	return core.HostImportDefaults{
		AuthRef:        strings.TrimSpace(opts.AuthRef),
		User:           firstNonEmpty(opts.User, configDefaults.User),
		KeyPath:        strings.TrimSpace(opts.KeyPath),
		Group:          group,
		Remark:         strings.TrimSpace(opts.Remark),
		Port:           port,
		Jump:           strings.TrimSpace(opts.Jump),
		Backend:        strings.TrimSpace(opts.Backend),
		Via:            strings.TrimSpace(opts.Via),
		RunTemplate:    strings.TrimSpace(opts.RunTemplate),
		LoginCommand:   strings.TrimSpace(opts.LoginCommand),
		HostKeyCheck:   hostKeyCheck,
		KnownHostsPath: knownHostsPath,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func writeHostImportPlan(c *gcli.Command, plan core.HostImportPlan, dryRun bool) {
	if dryRun {
		fmt.Fprintln(cmdOutput(c), "Dry-run: no changes saved")
	}
	fmt.Fprintf(cmdOutput(c), "Parsed: total=%d valid=%d invalid=%d\n", len(plan.Hosts), len(plan.Hosts)-len(plan.Invalid), len(plan.Invalid))
	fmt.Fprintf(cmdOutput(c), "Plan: add=%d update=%d skip=%d conflict=%d\n", plan.Added, plan.Updated, plan.Skipped, len(plan.Conflicts))
	for _, conflict := range plan.Conflicts {
		fmt.Fprintf(cmdOutput(c), "conflict: %s=%s %s\n", conflict.Field, conflict.Value, conflict.Reason)
	}
	writeHostImportErrors(c, "invalid", plan.Invalid)
}

func writeHostImportErrors(c *gcli.Command, prefix string, errs []core.HostImportError) {
	for _, item := range errs {
		fmt.Fprintf(cmdOutput(c), "%s: %s\n", prefix, item.Error())
	}
}

func newHostListCmd() *gcli.Command {
	opts := struct {
		ShowIP bool
		Group  string
		Match  string
		JSON   bool
	}{}
	return &gcli.Command{
		Name:    "list",
		Desc:    "list saved ssh hosts",
		Aliases: []string{"ls"},
		Config: func(c *gcli.Command) {
			c.BoolOpt(&opts.ShowIP, "show-ip", "", false, "show full host IP address")
			c.StrOpt(&opts.Group, "group", "", "", "filter by host group")
			c.StrOpt(&opts.Match, "match", "", "", "match host text")
			c.BoolOpt(&opts.JSON, "json", "", false, "output hosts as json")
		},
		Func: func(c *gcli.Command, _ []string) error {
			store, err := core.LoadStoreWithSSHConfig()
			if err != nil {
				return err
			}
			hosts := filterHosts(store.Hosts, opts.Group, opts.Match)
			if opts.JSON {
				return writeJSON(c, hosts)
			}
			out := buildHostListTable(hosts, opts.ShowIP)
			if out != "" {
				fmt.Fprint(cmdOutput(c), out)
			}
			return nil
		},
	}
}

func newHostShowCmd() *gcli.Command {
	var raw bool
	return &gcli.Command{
		Name: "show",
		Desc: "show ssh host",
		Config: func(c *gcli.Command) {
			c.BoolOpt(&raw, "raw", "", false, "show raw host from config")
			c.AddArg("target", "host ip or name", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			target := strings.TrimSpace(c.Arg("target").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			store := core.Store{Hosts: config.Hosts}
			host, ok, err := store.ResolveHost(target)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("host %q not found", target)
			}
			if !raw {
				host = core.MaskHost(host)
			}
			return writeJSON(c, host)
		},
	}
}

func newHostSetCmd() *gcli.Command {
	opts := hostSetOptions{}
	return &gcli.Command{
		Name: "set",
		Desc: "set ssh host fields",
		Config: func(c *gcli.Command) {
			c.AddArg("target", "host ip or name", true)
			c.StrOpt(&opts.IP, "ip", "", "", "ssh host ip or hostname")
			c.StrOpt(&opts.User, "user", "u", "", "ssh username")
			c.StrOpt(&opts.AuthRef, "auth", "", "", "auth profile name")
			c.StrOpt(&opts.KeyPath, "key", "", "", "ssh private key path")
			c.StrOpt(&opts.Remark, "remark", "", "", "host remark")
			c.StrOpt(&opts.Group, "group", "", "", "host group")
			c.StrOpt(&opts.Jump, "jump", "", "", "jump host name or ip")
			c.StrOpt(&opts.Backend, "backend", "", "", "host backend: ssh or command_proxy")
			c.StrOpt(&opts.Via, "via", "", "", "command_proxy via host name or ip")
			c.StrOpt(&opts.RunTemplate, "run-template", "", "", "command_proxy run template")
			c.StrOpt(&opts.LoginCommand, "login-command", "", "", "command_proxy login command")
			c.IntOpt(&opts.Port, "port", "", 0, "ssh port")
			c.StrOpt(&opts.ConnectTimeout, "connect-timeout", "", "", "ssh connect timeout")
			c.StrOpt(&opts.RunTimeout, "run-timeout", "", "", "remote command timeout")
			c.StrOpt(&opts.RemoteScriptDir, "remote-script-dir", "", "", "remote script directory")
			c.StrOpt(&opts.HostKeyCheck, "host-key-check", "", "", "host key check policy: known_hosts or insecure")
			c.StrOpt(&opts.KnownHostsPath, "known-hosts-path", "", "", "known_hosts file path")
		},
		Func: func(c *gcli.Command, _ []string) error {
			target := strings.TrimSpace(c.Arg("target").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			idx, host, err := findHostIndex(config.Hosts, target)
			if err != nil {
				return err
			}
			if idx < 0 {
				return fmt.Errorf("host %q not found", target)
			}
			changed := setHostFieldsFromOptions(&host, opts)
			if changed == 0 {
				return errors.New("no host field option provided")
			}
			if err := saveUpdatedHost(config, idx, host); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "updated host %s (%d fields)\n", core.HostLogName(host), changed)
			return nil
		},
	}
}

func newHostUnsetCmd() *gcli.Command {
	opts := hostUnsetOptions{}
	return &gcli.Command{
		Name: "unset",
		Desc: "unset ssh host fields",
		Config: func(c *gcli.Command) {
			c.AddArg("target", "host ip or name", true)
			c.BoolOpt(&opts.User, "user", "u", false, "unset ssh username")
			c.BoolOpt(&opts.AuthRef, "auth", "", false, "unset auth profile name")
			c.BoolOpt(&opts.KeyPath, "key", "", false, "unset ssh private key path")
			c.BoolOpt(&opts.Remark, "remark", "", false, "unset host remark")
			c.BoolOpt(&opts.Group, "group", "", false, "unset host group")
			c.BoolOpt(&opts.Jump, "jump", "", false, "unset jump host")
			c.BoolOpt(&opts.Backend, "backend", "", false, "unset host backend")
			c.BoolOpt(&opts.Via, "via", "", false, "unset command_proxy via host")
			c.BoolOpt(&opts.RunTemplate, "run-template", "", false, "unset command_proxy run template")
			c.BoolOpt(&opts.LoginCommand, "login-command", "", false, "unset command_proxy login command")
			c.BoolOpt(&opts.ConnectTimeout, "connect-timeout", "", false, "unset ssh connect timeout")
			c.BoolOpt(&opts.RunTimeout, "run-timeout", "", false, "unset remote command timeout")
			c.BoolOpt(&opts.RemoteScriptDir, "remote-script-dir", "", false, "unset remote script directory")
			c.BoolOpt(&opts.HostKeyCheck, "host-key-check", "", false, "unset host key check policy")
			c.BoolOpt(&opts.KnownHostsPath, "known-hosts-path", "", false, "unset known_hosts file path")
		},
		Func: func(c *gcli.Command, _ []string) error {
			target := strings.TrimSpace(c.Arg("target").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			idx, host, err := findHostIndex(config.Hosts, target)
			if err != nil {
				return err
			}
			if idx < 0 {
				return fmt.Errorf("host %q not found", target)
			}
			changed := unsetHostFieldsFromOptions(&host, opts)
			if changed == 0 {
				return errors.New("no host field option provided")
			}
			if err := saveUpdatedHost(config, idx, host); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "updated host %s (%d fields)\n", core.HostLogName(host), changed)
			return nil
		},
	}
}

func newHostRemoveCmd() *gcli.Command {
	var yes bool
	return &gcli.Command{
		Name:    "rm",
		Desc:    "remove ssh host",
		Aliases: []string{"remove", "delete"},
		Config: func(c *gcli.Command) {
			c.BoolOpt(&yes, "yes", "y", false, "confirm removal")
			c.AddArg("target", "host ip or name", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			target := strings.TrimSpace(c.Arg("target").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			idx, host, err := findHostIndex(config.Hosts, target)
			if err != nil {
				return err
			}
			if idx < 0 {
				return fmt.Errorf("host %q not found", target)
			}
			if !yes {
				if ok, err := confirmInteractive(fmt.Sprintf("remove host %s?", core.HostLogName(host))); err != nil {
					return err
				} else if !ok {
					return errors.New("remove canceled")
				}
			}
			config.Hosts = append(config.Hosts[:idx], config.Hosts[idx+1:]...)
			if err := core.SaveConfig(config); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "removed host %s\n", core.HostLogName(host))
			return nil
		},
	}
}

func newHostRenameCmd() *gcli.Command {
	return &gcli.Command{
		Name: "rename",
		Desc: "rename ssh host",
		Config: func(c *gcli.Command) {
			c.AddArg("old_name", "old host name", true)
			c.AddArg("new_name", "new host name", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			oldName := strings.TrimSpace(c.Arg("old_name").String())
			newName := strings.TrimSpace(c.Arg("new_name").String())
			if newName == "" {
				return errors.New("new host name is required")
			}
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			if idx, _, err := findHostIndex(config.Hosts, newName); err != nil {
				return err
			} else if idx >= 0 {
				return fmt.Errorf("host %q already exists", newName)
			}
			idx, _, err := findHostIndex(config.Hosts, oldName)
			if err != nil {
				return err
			}
			if idx < 0 {
				return fmt.Errorf("host %q not found", oldName)
			}
			config.Hosts[idx].Name = newName
			if err := core.SaveConfig(config); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "renamed host %s to %s\n", oldName, newName)
			return nil
		},
	}
}

func filterHosts(hosts []core.Host, group, match string) []core.Host {
	group = strings.TrimSpace(group)
	match = strings.TrimSpace(match)
	if group == "" && match == "" {
		return hosts
	}
	store := core.Store{Hosts: hosts}
	matched := hosts
	if match != "" {
		matched = store.MatchHosts(match)
	}
	if group == "" {
		return matched
	}
	filtered := make([]core.Host, 0, len(matched))
	for _, host := range matched {
		if core.HostGroupName(host) == group {
			filtered = append(filtered, host)
		}
	}
	return filtered
}

func setHostFieldsFromOptions(host *core.Host, opts hostSetOptions) int {
	changed := 0
	setString := func(dst *string, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		*dst = value
		changed++
	}
	setString(&host.IP, opts.IP)
	setString(&host.User, opts.User)
	setString(&host.AuthRef, opts.AuthRef)
	setString(&host.KeyPath, opts.KeyPath)
	setString(&host.Remark, opts.Remark)
	setString(&host.Group, opts.Group)
	setString(&host.Jump, opts.Jump)
	setString(&host.Backend, opts.Backend)
	setString(&host.Via, opts.Via)
	setString(&host.RunTemplate, opts.RunTemplate)
	setString(&host.LoginCommand, opts.LoginCommand)
	setString(&host.ConnectTimeout, opts.ConnectTimeout)
	setString(&host.RunTimeout, opts.RunTimeout)
	setString(&host.RemoteScriptDir, opts.RemoteScriptDir)
	setString(&host.KnownHostsPath, opts.KnownHostsPath)
	if value := strings.TrimSpace(opts.HostKeyCheck); value != "" {
		host.HostKeyCheck = value
		changed++
	}
	if opts.Port != 0 {
		host.Port = opts.Port
		changed++
	}
	return changed
}

func unsetHostFieldsFromOptions(host *core.Host, opts hostUnsetOptions) int {
	changed := 0
	unsetString := func(enabled bool, dst *string) {
		if !enabled {
			return
		}
		*dst = ""
		changed++
	}
	unsetString(opts.User, &host.User)
	unsetString(opts.AuthRef, &host.AuthRef)
	unsetString(opts.KeyPath, &host.KeyPath)
	unsetString(opts.Remark, &host.Remark)
	unsetString(opts.Group, &host.Group)
	unsetString(opts.Jump, &host.Jump)
	unsetString(opts.Backend, &host.Backend)
	unsetString(opts.Via, &host.Via)
	unsetString(opts.RunTemplate, &host.RunTemplate)
	unsetString(opts.LoginCommand, &host.LoginCommand)
	unsetString(opts.ConnectTimeout, &host.ConnectTimeout)
	unsetString(opts.RunTimeout, &host.RunTimeout)
	unsetString(opts.RemoteScriptDir, &host.RemoteScriptDir)
	unsetString(opts.HostKeyCheck, &host.HostKeyCheck)
	unsetString(opts.KnownHostsPath, &host.KnownHostsPath)
	return changed
}

func saveUpdatedHost(config *core.Config, idx int, host core.Host) error {
	if idx < 0 || idx >= len(config.Hosts) {
		return errors.New("host index out of range")
	}
	updated := *config
	updated.Hosts = append([]core.Host(nil), config.Hosts...)
	updated.Hosts[idx] = host
	if err := validateHostUniqueness(updated.Hosts, idx); err != nil {
		return err
	}
	if _, _, err := updated.EffectiveHost(host, core.HostOverrides{}); err != nil {
		return err
	}
	if core.IsCommandProxyHost(host) {
		if issues := core.CheckConfig(updated); core.HasDoctorErrors(issues) {
			return errors.New(formatDoctorErrors(issues))
		}
	} else if jump := strings.TrimSpace(host.Jump); jump != "" {
		if _, err := updated.ResolveConnection(host); err != nil {
			return err
		}
	}
	config.Hosts = updated.Hosts
	return core.SaveConfig(config)
}

func formatDoctorErrors(issues []core.DoctorIssue) string {
	var messages []string
	for _, issue := range issues {
		if issue.Level == core.DoctorError {
			messages = append(messages, issue.Message)
		}
	}
	if len(messages) == 0 {
		return "config doctor found errors"
	}
	return strings.Join(messages, "; ")
}

func validateHostUniqueness(hosts []core.Host, current int) error {
	host := hosts[current]
	for i, item := range hosts {
		if i == current {
			continue
		}
		if strings.TrimSpace(host.Name) != "" && strings.TrimSpace(host.Name) == strings.TrimSpace(item.Name) {
			return fmt.Errorf("host %q already exists", host.Name)
		}
		if strings.TrimSpace(host.IP) != "" && strings.TrimSpace(host.IP) == strings.TrimSpace(item.IP) {
			return fmt.Errorf("host ip %q already exists", host.IP)
		}
	}
	return nil
}

func findHostIndex(hosts []core.Host, target string) (int, core.Host, error) {
	store := core.Store{Hosts: hosts}
	host, ok, err := store.ResolveHost(target)
	if err != nil {
		return -1, core.Host{}, err
	}
	if !ok {
		return -1, core.Host{}, nil
	}
	for i, item := range hosts {
		if item.Name == host.Name && item.IP == host.IP {
			return i, item, nil
		}
	}
	return -1, core.Host{}, nil
}
