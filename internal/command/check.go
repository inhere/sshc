package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gookit/cliui/show/table"
	"github.com/gookit/gcli/v3"
	"github.com/inhere/sshc/internal/core"
)

type checkOptions struct {
	Hosts    string
	Group    string
	Tag      string
	All      bool
	Parallel int
	JSON     bool
	Timeout  string
}

func NewCheckCmd() *gcli.Command {
	opts := &checkOptions{Parallel: 5}
	return &gcli.Command{
		Name: "check",
		Desc: "check host connectivity",
		Help: strings.TrimSpace(`
Examples:
  sshc check devhost
  sshc check --hosts devhost,dbhost
  sshc check --group testing
  sshc check --tag app
  sshc check --all --parallel 10
  sshc check --json --tag app

Notes:
  - check verifies host resolution, local key files, known_hosts path, TCP, SSH handshake, and auth.
  - command_proxy targets check local configuration only; their via host should be checked separately.
`),
		Config: func(c *gcli.Command) {
			c.StrOpt(&opts.Hosts, "hosts", "", "", "comma-separated host names or IPs")
			c.StrOpt(&opts.Group, "group", "", "", "filter by host group")
			c.StrOpt(&opts.Tag, "tag", "", "", "filter by comma-separated host tags")
			c.BoolOpt(&opts.All, "all", "", false, "check all saved hosts")
			c.IntOpt(&opts.Parallel, "parallel", "", 5, "max hosts to check at once")
			c.BoolOpt(&opts.JSON, "json", "", false, "output check results as json")
			c.StrOpt(&opts.Timeout, "timeout", "", "", "connect timeout, eg: 5s or bare seconds")
			c.AddArg("target", "host ip or name", false)
		},
		Func: func(c *gcli.Command, _ []string) error {
			if opts.Parallel < 1 {
				return errors.New("--parallel must be greater than 0")
			}
			timeout, err := parseCheckTimeout(opts.Timeout)
			if err != nil {
				return err
			}
			config, err := core.LoadConfigWithSSHConfig()
			if err != nil {
				return err
			}
			hosts, err := resolveCheckHosts(*config, strings.TrimSpace(c.Arg("target").String()), *opts)
			if err != nil {
				return err
			}
			results := runHostChecks(hosts, core.CheckOptions{Timeout: timeout}, opts.Parallel)
			if opts.JSON {
				if err := writeCheckJSON(c, results); err != nil {
					return err
				}
			} else if out := buildCheckTable(results); out != "" {
				fmt.Fprint(cmdOutput(c), out)
			}
			if checkHasFailures(results) {
				return errors.New("check failed")
			}
			return nil
		},
	}
}

func resolveCheckHosts(config core.Config, target string, opts checkOptions) ([]core.Host, error) {
	sourceCount := 0
	if target != "" {
		sourceCount++
	}
	if strings.TrimSpace(opts.Hosts) != "" {
		sourceCount++
	}
	if strings.TrimSpace(opts.Group) != "" {
		sourceCount++
	}
	if strings.TrimSpace(opts.Tag) != "" {
		sourceCount++
	}
	if opts.All {
		sourceCount++
	}
	if sourceCount != 1 {
		return nil, errors.New("exactly one of target, --hosts, --group, --tag, or --all is required")
	}
	store := core.Store{Hosts: config.Hosts}
	switch {
	case target != "":
		host, ok, err := store.ResolveHost(target)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("host %q not found", target)
		}
		return effectiveCheckHosts(config, []core.Host{host})
	case strings.TrimSpace(opts.Hosts) != "":
		var hosts []core.Host
		for _, target := range strings.Split(opts.Hosts, ",") {
			target = strings.TrimSpace(target)
			if target == "" {
				return nil, errors.New("empty host target is not allowed")
			}
			host, ok, err := store.ResolveHost(target)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("host %q not found", target)
			}
			hosts = append(hosts, host)
		}
		return effectiveCheckHosts(config, hosts)
	case strings.TrimSpace(opts.Group) != "":
		return effectiveCheckHosts(config, filterHosts(config.Hosts, opts.Group, "", ""))
	case strings.TrimSpace(opts.Tag) != "":
		return effectiveCheckHosts(config, filterHosts(config.Hosts, "", opts.Tag, ""))
	default:
		return effectiveCheckHosts(config, config.Hosts)
	}
}

func effectiveCheckHosts(config core.Config, hosts []core.Host) ([]core.Host, error) {
	if len(hosts) == 0 {
		return nil, errors.New("no hosts found")
	}
	effective := make([]core.Host, 0, len(hosts))
	for _, host := range hosts {
		resolved, _, err := config.EffectiveHost(host, core.HostOverrides{})
		if err != nil {
			return nil, err
		}
		effective = append(effective, resolved.ToHost())
	}
	return effective, nil
}

func runHostChecks(hosts []core.Host, opts core.CheckOptions, parallel int) []core.CheckResult {
	if parallel > len(hosts) {
		parallel = len(hosts)
	}
	results := make([]core.CheckResult, len(hosts))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				results[idx] = core.CheckHost(hosts[idx], opts)
			}
		}()
	}
	for idx := range hosts {
		jobs <- idx
	}
	close(jobs)
	wg.Wait()
	return results
}

func buildCheckTable(results []core.CheckResult) string {
	if len(results) == 0 {
		return ""
	}
	tb := table.New("", table.WithBorderFlags(table.BorderDefault), table.WithOverflowFlag(table.OverflowWrap))
	tb.SetHeads("Name", "Group", "Tags", "Addr", "TCP", "SSH", "Auth", "HostKey", "Latency", "Error")
	for _, result := range results {
		tb.AddRow(result.Name, result.Group, strings.Join(result.Tags, ","), result.Address, result.TCP, result.SSH, result.Auth, result.HostKey, checkLatencyLabel(result.LatencyMS), checkErrorLabel(result.Error))
	}
	return tb.String()
}

func checkLatencyLabel(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	return fmt.Sprintf("%dms", ms)
}

func checkErrorLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func checkHasFailures(results []core.CheckResult) bool {
	for _, result := range results {
		if strings.TrimSpace(result.Error) != "" || result.TCP == core.CheckStatusFail || result.SSH == core.CheckStatusFail || result.Auth == core.CheckStatusFail || result.HostKey == core.CheckStatusFail {
			return true
		}
	}
	return false
}

func parseCheckTimeout(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return core.ParseTimeout(value)
}

func writeCheckJSON(c *gcli.Command, results []core.CheckResult) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmdOutput(c), string(data))
	return nil
}
