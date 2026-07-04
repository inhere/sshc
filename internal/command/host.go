package command

import (
	"errors"
	"fmt"
	"strings"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/gcli/v3"
)

func NewHostCmd() *gcli.Command {
	cmd := &gcli.Command{
		Name:     "host",
		Desc:     "manage ssh hosts",
		Aliases:  []string{"hosts", "h"},
		Category: managementCategory,
		Help: strings.TrimSpace(`
Examples:
  sshc host add --ip 192.168.1.10 --name devhost --auth dev-root
  sshc host list
  sshc host list --group testing
  sshc host list --match devhost
  sshc host list --show-ip
  sshc host show devhost
  sshc host rm devhost --yes
  sshc host rename old-name new-name

Notes:
  - host add supports the same fields as top-level add and adds --auth.
  - host list masks IPv4 addresses unless --show-ip is set.
  - host show masks password fields unless --raw is set.
`),
	}
	cmd.Add(
		newHostAddCmd(),
		newHostListCmd(),
		newHostShowCmd(),
		newHostRemoveCmd(),
		newHostRenameCmd(),
	)
	return cmd
}

func newHostAddCmd() *gcli.Command {
	cmd := NewAddCmd()
	cmd.Name = "add"
	cmd.Aliases = nil
	cmd.Desc = "add or update ssh host"
	return cmd
}

func newHostListCmd() *gcli.Command {
	opts := struct {
		ShowIP bool
		Group  string
		Match  string
		JSON   bool
	}{}
	return &gcli.Command{
		Name: "list",
		Desc: "list saved ssh hosts",
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
