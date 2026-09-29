package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/cliui/show/table"
	"github.com/gookit/gcli/v3"
)

// Command-layer seams, overridable from hooks_test.go.
var (
	startLocalForward = core.StartLocalForward
	notifyContext     = signal.NotifyContext
)

type tunnelFlags struct {
	Target         string
	Address        string
	Forwards       gcli.Strings
	AuthRef        string
	Port           int
	Jump           string
	ConnectTimeout string
	JSON           bool
	Verbose        bool
	Quiet          bool
	Force          bool
	Yes            bool
}

func (f *tunnelFlags) bind(c *gcli.Command) {
	c.StrOpt(&f.Target, "target", "", "", "saved host name or ip")
	c.StrOpt(&f.Address, "address", "", "", "unregistered ip or hostname (requires --auth)")
	c.VarOpt(&f.Forwards, "forward", "", "forward rule local=remote, repeatable")
	c.StrOpt(&f.AuthRef, "auth", "", "", "auth profile for the target")
	c.IntOpt(&f.Port, "port", "", 0, "ssh port")
	c.StrOpt(&f.Jump, "jump", "", "", "jump host name")
	c.StrOpt(&f.ConnectTimeout, "connect-timeout", "", "", "ssh connect timeout, eg: 10s")
}

// snapshot copies the parsed flags and arms a reset, so repeated in-process runs
// (tests, embedded use) do not accumulate repeatable values like --forward.
func (f *tunnelFlags) snapshot() (tunnelFlags, func()) {
	copyOf := *f
	return copyOf, f.reset
}

func (f *tunnelFlags) reset() {
	f.Target = ""
	f.Address = ""
	f.Forwards = nil
	f.AuthRef = ""
	f.Port = 0
	f.Jump = ""
	f.ConnectTimeout = ""
	f.JSON = false
	f.Verbose = false
	f.Quiet = false
	f.Force = false
	f.Yes = false
}

func NewTunnelCmd() *gcli.Command {
	cmd := &gcli.Command{
		Name:     "tunnel",
		Desc:     "Manage local port forwarding tunnels",
		Aliases:  []string{"tun"},
		Category: managementCategory,
		Help: strings.TrimSpace(`
Examples:
  sshc tunnel add dev-db --target devhost --forward 15432=127.0.0.1:5432
  sshc tunnel add dev-stack --target devhost --forward 15432=5432 --forward 16379=6379
  sshc tunnel add prod-redis --address 192.168.1.20 --auth dev-root --port 2222 --forward 16379=6379
  sshc tunnel list
  sshc tunnel show dev-db
  sshc tunnel forward dev-db
  sshc tun forward --target devhost --forward 15433=127.0.0.1:5432
  sshc tunnel rm dev-db --yes

Notes:
  - --target accepts a saved host; --address accepts an unregistered address and requires --auth.
  - Forward rules use local=remote, eg: 15432=127.0.0.1:5432; a bare port means 127.0.0.1:port.
  - Only loopback local endpoints are allowed; local port 0 lets the OS pick a free port.
  - tunnel forward stays in the foreground; Ctrl-C closes listeners and the ssh session.
`),
	}
	cmd.Add(
		newTunnelAddCmd(),
		newTunnelListCmd(),
		newTunnelShowCmd(),
		newTunnelRemoveCmd(),
		newTunnelForwardCmd(),
	)
	return cmd
}

func newTunnelAddCmd() *gcli.Command {
	opts := &tunnelFlags{}
	return &gcli.Command{
		Name: "add",
		Desc: "create a saved tunnel profile",
		Config: func(c *gcli.Command) {
			opts.bind(c)
			c.BoolOpt(&opts.Force, "force", "", false, "overwrite an existing tunnel")
			c.AddArg("name", "tunnel name", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			flags, reset := opts.snapshot()
			defer reset()
			name := strings.TrimSpace(c.Arg("name").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			profile, err := buildTunnelProfile(*config, name, &flags)
			if err != nil {
				return err
			}
			profiles, err := core.UpsertTunnel(config.Tunnels, profile, flags.Force)
			if err != nil {
				return err
			}
			config.Tunnels = profiles
			if issues := core.CheckConfig(*config); core.HasDoctorErrors(issues) {
				return fmt.Errorf("invalid tunnel config: %s", formatDoctorErrors(issues))
			}
			if err := core.SaveConfig(config); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "saved tunnel %s (%s)\n", profile.Name, tunnelTargetLabel(profile))
			return nil
		},
	}
}

func newTunnelListCmd() *gcli.Command {
	var asJSON bool
	return &gcli.Command{
		Name:    "list",
		Desc:    "list saved tunnels",
		Aliases: []string{"ls"},
		Config: func(c *gcli.Command) {
			c.BoolOpt(&asJSON, "json", "", false, "output json")
		},
		Func: func(c *gcli.Command, _ []string) error {
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSONOutput(c, config.Tunnels)
			}
			if len(config.Tunnels) == 0 {
				fmt.Fprintln(cmdOutput(c), "no tunnels saved")
				return nil
			}
			fmt.Fprint(cmdOutput(c), buildTunnelListTable(config.Tunnels))
			return nil
		},
	}
}

func newTunnelShowCmd() *gcli.Command {
	var asJSON bool
	return &gcli.Command{
		Name: "show",
		Desc: "show one saved tunnel",
		Config: func(c *gcli.Command) {
			c.BoolOpt(&asJSON, "json", "", false, "output json")
			c.AddArg("name", "tunnel name", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			name := strings.TrimSpace(c.Arg("name").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			profile, ok := core.FindTunnel(config.Tunnels, name)
			if !ok {
				return fmt.Errorf("tunnel %q not found", name)
			}
			if asJSON {
				return writeJSONOutput(c, profile)
			}
			out := cmdOutput(c)
			fmt.Fprintf(out, "name: %s\n", profile.Name)
			fmt.Fprintf(out, "target: %s\n", firstNonEmpty(profile.Target, "-"))
			fmt.Fprintf(out, "address: %s\n", firstNonEmpty(profile.Address, "-"))
			fmt.Fprintf(out, "port: %s\n", tunnelPortLabel(profile.Port))
			fmt.Fprintf(out, "jump: %s\n", firstNonEmpty(profile.Jump, "-"))
			fmt.Fprintf(out, "auth: %s\n", firstNonEmpty(profile.AuthRef, "-"))
			fmt.Fprintf(out, "remark: %s\n", firstNonEmpty(profile.Remark, "-"))
			for _, rule := range profile.Forwards {
				fmt.Fprintf(out, "forward: %s -> %s\n", rule.Local, rule.Remote)
			}
			return nil
		},
	}
}

func newTunnelRemoveCmd() *gcli.Command {
	opts := &tunnelFlags{}
	return &gcli.Command{
		Name:    "rm",
		Desc:    "remove a saved tunnel",
		Aliases: []string{"remove", "delete"},
		Config: func(c *gcli.Command) {
			c.BoolOpt(&opts.Yes, "yes", "y", false, "confirm removal")
			c.AddArg("name", "tunnel name", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			flags, reset := opts.snapshot()
			defer reset()
			name := strings.TrimSpace(c.Arg("name").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			if _, ok := core.FindTunnel(config.Tunnels, name); !ok {
				return fmt.Errorf("tunnel %q not found", name)
			}
			if !flags.Yes {
				if ok, err := confirmInteractive(fmt.Sprintf("remove tunnel %s?", name)); err != nil {
					return err
				} else if !ok {
					return errors.New("remove canceled")
				}
			}
			config.Tunnels = core.RemoveTunnel(config.Tunnels, name)
			if err := core.SaveConfig(config); err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "removed tunnel %s\n", name)
			return nil
		},
	}
}

func newTunnelForwardCmd() *gcli.Command {
	opts := &tunnelFlags{}
	return &gcli.Command{
		Name:    "forward",
		Desc:    "run a tunnel in the foreground",
		Aliases: []string{"start"},
		Config: func(c *gcli.Command) {
			opts.bind(c)
			c.BoolOpt(&opts.JSON, "json", "", false, "print the ready payload as one json line on stdout")
			c.BoolOpt(&opts.Verbose, "verbose", "", false, "log per-connection details")
			c.BoolOpt(&opts.Quiet, "quiet", "", false, "suppress non-error diagnostics")
			c.AddArg("name", "tunnel name", false)
		},
		Func: func(c *gcli.Command, _ []string) error {
			flags, reset := opts.snapshot()
			defer reset()
			name := strings.TrimSpace(c.Arg("name").String())
			config, err := core.LoadConfig()
			if err != nil {
				return err
			}
			profile, err := resolveForwardTunnelProfile(*config, name, &flags)
			if err != nil {
				return err
			}
			host, err := core.ResolveTunnelHost(*config, profile)
			if err != nil {
				return err
			}
			rules, err := profile.ForwardRules()
			if err != nil {
				return err
			}
			options, err := buildForwardOptions(&flags)
			if err != nil {
				return err
			}
			session, err := startLocalForward(host, rules, options)
			if err != nil {
				return err
			}
			if err := writeTunnelReady(c, &flags, profile, session.Endpoints(), rules); err != nil {
				_ = session.Close()
				return err
			}

			ctx, stop := notifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			go func() {
				<-ctx.Done()
				_ = session.Close()
			}()
			return session.Wait()
		},
	}
}

// resolveForwardTunnelProfile merges a saved profile with one-off CLI overrides.
func resolveForwardTunnelProfile(config core.Config, name string, opts *tunnelFlags) (core.TunnelProfile, error) {
	target := strings.TrimSpace(opts.Target)
	address := strings.TrimSpace(opts.Address)
	if target != "" && address != "" {
		return core.TunnelProfile{}, errors.New("--target and --address are mutually exclusive")
	}

	var profile core.TunnelProfile
	if name != "" {
		saved, ok := core.FindTunnel(config.Tunnels, name)
		if !ok {
			return core.TunnelProfile{}, fmt.Errorf("tunnel %q not found", name)
		}
		profile = saved
	}
	profile.Name = name
	if target != "" {
		profile.Target = target
		profile.Address = ""
	}
	if address != "" {
		profile.Address = address
		profile.Target = ""
	}
	if ref := strings.TrimSpace(opts.AuthRef); ref != "" {
		profile.AuthRef = ref
	}
	if jump := strings.TrimSpace(opts.Jump); jump != "" {
		profile.Jump = jump
	}
	if opts.Port > 0 {
		profile.Port = opts.Port
	}
	if len(opts.Forwards) > 0 {
		forwardArgs := opts.Forwards.Strings()
		rules := make([]core.TunnelForward, 0, len(forwardArgs))
		for _, raw := range forwardArgs {
			rule, err := core.ParseForwardRule(raw)
			if err != nil {
				return core.TunnelProfile{}, err
			}
			rules = append(rules, rule)
		}
		profile.Forwards = rules
	}
	core.NormalizeTunnelProfile(&profile)
	if profile.Target == "" && profile.Address == "" {
		return core.TunnelProfile{}, errors.New("--target or --address is required")
	}
	if len(profile.Forwards) == 0 {
		return core.TunnelProfile{}, errors.New("at least one --forward is required")
	}
	return profile, nil
}

func buildTunnelProfile(config core.Config, name string, opts *tunnelFlags) (core.TunnelProfile, error) {
	profile := core.TunnelProfile{
		Name:    name,
		AuthRef: strings.TrimSpace(opts.AuthRef),
		Jump:    strings.TrimSpace(opts.Jump),
		Port:    opts.Port,
	}
	target := strings.TrimSpace(opts.Target)
	address := strings.TrimSpace(opts.Address)
	switch {
	case target != "" && address != "":
		return profile, errors.New("--target and --address are mutually exclusive")
	case target == "" && address == "":
		return profile, errors.New("--target or --address is required")
	case address != "":
		profile.Address = address
	default:
		canonical, err := core.CanonicalTunnelTarget(config, target)
		if err != nil {
			return profile, err
		}
		profile.Target = canonical
	}
	for _, raw := range opts.Forwards.Strings() {
		rule, err := core.ParseForwardRule(raw)
		if err != nil {
			return profile, err
		}
		profile.Forwards = append(profile.Forwards, rule)
	}
	core.NormalizeTunnelProfile(&profile)
	if err := core.ValidateTunnelProfile(config, profile); err != nil {
		return profile, err
	}
	return profile, nil
}

func buildForwardOptions(opts *tunnelFlags) (core.ForwardOptions, error) {
	options := core.ForwardOptions{Logf: tunnelLogger(opts)}
	if value := strings.TrimSpace(opts.ConnectTimeout); value != "" {
		timeout, err := core.ParseTimeout(value)
		if err != nil {
			return options, fmt.Errorf("invalid --connect-timeout: %w", err)
		}
		options.ConnectTimeout = timeout
	}
	return options, nil
}

func tunnelLogger(opts *tunnelFlags) func(string, ...any) {
	if !opts.Verbose || opts.Quiet {
		return nil
	}
	return func(format string, args ...any) {
		fmt.Fprintf(statusOutput, "tunnel: "+format+"\n", args...)
	}
}

// writeTunnelReady emits the ready payload: one json line on stdout, or a human
// line per listener on stderr.
func writeTunnelReady(c *gcli.Command, opts *tunnelFlags, profile core.TunnelProfile, endpoints []string, rules []core.ForwardRule) error {
	if len(endpoints) != len(rules) {
		return fmt.Errorf("listener count %d does not match rule count %d", len(endpoints), len(rules))
	}
	if opts.JSON {
		type listenerPayload struct {
			Local  string `json:"local"`
			Remote string `json:"remote"`
		}
		payload := struct {
			Name      string            `json:"name"`
			Target    string            `json:"target,omitempty"`
			Address   string            `json:"address,omitempty"`
			Listeners []listenerPayload `json:"listeners"`
		}{Name: profile.Name, Target: profile.Target, Address: profile.Address, Listeners: []listenerPayload{}}
		for i := range endpoints {
			payload.Listeners = append(payload.Listeners, listenerPayload{Local: endpoints[i], Remote: rules[i].RemoteAddr})
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmdOutput(c), string(data))
		return nil
	}
	if opts.Quiet {
		return nil
	}
	for i := range endpoints {
		fmt.Fprintf(cmdStatus(c), "tunnel ready %s -> %s\n", endpoints[i], rules[i].RemoteAddr)
	}
	return nil
}

func buildTunnelListTable(profiles []core.TunnelProfile) string {
	tb := table.New("", table.WithBorderFlags(table.BorderDefault), table.WithOverflowFlag(table.OverflowWrap))
	tb.SetHeads("Name", "Mode", "Target", "Port", "Jump", "Forwards", "Local", "Remark")
	for _, profile := range profiles {
		tb.AddRow(
			profile.Name,
			tunnelMode(profile),
			firstNonEmpty(profile.Target, profile.Address, "-"),
			tunnelPortLabel(profile.Port),
			firstNonEmpty(profile.Jump, "-"),
			len(profile.Forwards),
			tunnelLocalPorts(profile),
			firstNonEmpty(profile.Remark, "-"),
		)
	}
	return tb.String()
}

func tunnelMode(profile core.TunnelProfile) string {
	if strings.TrimSpace(profile.Address) != "" {
		return "address"
	}
	return "host"
}

func tunnelPortLabel(port int) string {
	if port == 0 {
		return "22"
	}
	return fmt.Sprintf("%d", port)
}

func tunnelLocalPorts(profile core.TunnelProfile) string {
	ports := make([]string, 0, len(profile.Forwards))
	for _, rule := range profile.Forwards {
		ports = append(ports, rule.Local)
	}
	return strings.Join(ports, ",")
}

func tunnelTargetLabel(profile core.TunnelProfile) string {
	if strings.TrimSpace(profile.Address) != "" {
		return fmt.Sprintf("address %s:%s", profile.Address, tunnelPortLabel(profile.Port))
	}
	return fmt.Sprintf("target %s:%s", profile.Target, tunnelPortLabel(profile.Port))
}

func writeJSONOutput(c *gcli.Command, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmdOutput(c), string(data))
	return nil
}
