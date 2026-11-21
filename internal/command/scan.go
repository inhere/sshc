package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gookit/cliui/show/table"
	"github.com/gookit/gcli/v3"
	"github.com/inhere/sshc/internal/core"
)

const (
	defaultScanTimeout  = 400 * time.Millisecond
	maxScanTimeout      = 10 * time.Second
	defaultScanParallel = 128
	maxScanParallel     = 1024
)

// Replacable for tests: the command never touches the network on its own.
var (
	scanBuildTargets  = core.BuildScanTargets
	scanReadNeighbors = core.ReadIPv6Neighbors
	scanProbeSSH      = core.ProbeSSH
	scanNotifyContext = signalScanContext
)

type scanOptions struct {
	CIDRs      gcli.Strings
	Interfaces gcli.Strings
	Ports      gcli.Strings
	Timeout    string
	Parallel   int
	MaxHosts   int
	JSON       bool
	Force      bool
	Save       bool
	Group      string
}

func NewScanCmd() *gcli.Command {
	opts := &scanOptions{Timeout: "400ms", Parallel: defaultScanParallel, MaxHosts: core.DefaultScanMaxHosts, Group: "lan"}
	cmd := &gcli.Command{
		Name:    "scan",
		Aliases: []string{"discover"},
		Desc:    "discover ssh servers on directly connected networks",
		Help: strings.TrimSpace(`
Examples:
  sshc scan
  sshc scan --cidr 192.168.1.0/24 --json
  sshc scan --interface 12 --timeout 300ms --parallel 64
  sshc scan --save --group sw1

Notes:
  - Without --cidr the local IPv4 networks and the system IPv6 neighbor cache are used.
  - Only private, link-local and loopback networks are scanned unless --force is given.
  - A target counts as SSH only when it answers with an SSH- identification line.
  - IPv6 discovery reads the neighbor cache; use --cidr or ping6 ff02::1%<if> to prime it.
  - --save only appends name, ip, port and group; no credentials are written.
`),
		Config: func(c *gcli.Command) {
			c.SetValidateNum(true)
			c.VarOpt(&opts.CIDRs, "cidr", "", "ipv4/ipv6 cidr to scan, repeatable")
			c.VarOpt(&opts.Interfaces, "interface", "", "limit discovery to one interface name or index, repeatable")
			c.VarOpt(&opts.Ports, "port", "", "ssh port to probe, repeatable (default 22)")
			c.StrOpt(&opts.Timeout, "timeout", "", "400ms", "connect and banner timeout, eg: 300ms, 2s")
			c.IntOpt(&opts.Parallel, "parallel", "", defaultScanParallel, "max concurrent probes")
			c.IntOpt(&opts.MaxHosts, "max-hosts", "", core.DefaultScanMaxHosts, "max addresses to probe in one run")
			c.BoolOpt(&opts.JSON, "json", "", false, "output scan results as json")
			c.BoolOpt(&opts.Force, "force", "", false, "allow scanning non private networks")
			c.BoolOpt(&opts.Save, "save", "", false, "save discovered hosts to the config")
			c.StrOpt(&opts.Group, "group", "", "lan", "host group for --save")
		},
		Func: func(c *gcli.Command, _ []string) error {
			return runScan(c, *opts)
		},
	}
	return cmd
}

func runScan(c *gcli.Command, opts scanOptions) error {
	timeout, err := parseScanTimeout(opts.Timeout)
	if err != nil {
		return err
	}
	if opts.Parallel < 1 || opts.Parallel > maxScanParallel {
		return fmt.Errorf("--parallel must be between 1 and %d", maxScanParallel)
	}
	if opts.MaxHosts < 1 {
		return errors.New("--max-hosts must be greater than 0")
	}
	ports, err := parseScanPorts(opts.Ports)
	if err != nil {
		return err
	}
	group := strings.TrimSpace(opts.Group)
	if opts.Save && group == "" {
		return errors.New("--save requires a non-empty --group")
	}

	targets, statuses, err := scanBuildTargets(opts.CIDRs, opts.Interfaces, core.ScanOptions{
		MaxHosts: opts.MaxHosts,
		Force:    opts.Force,
	})
	if err != nil {
		return err
	}
	if len(opts.CIDRs) == 0 {
		neighbors, status := scanReadNeighbors()
		statuses = append(statuses, status)
		targets = append(targets, neighbors...)
	}
	if len(targets) == 0 {
		fmt.Fprintln(cmdStatus(c), "sshc: no scannable targets found; pass --cidr to name a network")
		return nil
	}

	ctx, stop := scanNotifyContext()
	defer stop()

	results := make([]core.ScanResult, 0, len(targets))
	var probeErr error
	for _, port := range ports {
		part, err := scanProbeSSH(ctx, targets, port, timeout, opts.Parallel, nil)
		results = append(results, part...)
		if err != nil {
			probeErr = err
			break
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Address < results[j].Address })

	if opts.JSON {
		if err := writeScanJSON(c, results); err != nil {
			return err
		}
	} else {
		writeScanTable(c, results)
		for _, status := range statuses {
			if !status.Available {
				fmt.Fprintf(cmdStatus(c), "sshc: %s source unavailable: %s\n", status.Source, status.Detail)
			}
		}
		fmt.Fprintf(cmdStatus(c), "sshc: %d ssh server(s) of %d target(s)\n", len(results), len(targets))
	}

	if opts.Save {
		saved, skipped, err := saveScanHosts(results, group)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmdStatus(c), "sshc: saved: %d, skipped: %d\n", saved, skipped)
	}

	if probeErr != nil {
		return fmt.Errorf("scan interrupted: %w", probeErr)
	}
	return nil
}

func parseScanTimeout(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultScanTimeout, nil
	}
	timeout, err := core.ParseTimeout(value)
	if err != nil {
		return 0, err
	}
	if timeout <= 0 {
		return defaultScanTimeout, nil
	}
	if timeout > maxScanTimeout {
		return 0, fmt.Errorf("--timeout must not exceed %s", maxScanTimeout)
	}
	return timeout, nil
}

// parseScanPorts returns the probe ports, defaulting to the SSH port.
func parseScanPorts(values []string) ([]int, error) {
	if len(values) == 0 {
		return []int{core.DefaultSSHPort}, nil
	}
	seen := make(map[int]struct{}, len(values))
	ports := make([]int, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("--port is empty")
		}
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid port %q, want 1-65535", value)
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	return ports, nil
}

func writeScanTable(c *gcli.Command, results []core.ScanResult) {
	if len(results) == 0 {
		return
	}
	tb := table.New("", table.WithBorderFlags(table.BorderDefault), table.WithOverflowFlag(table.OverflowWrap))
	tb.SetHeads("Addr", "Zone", "Port", "Banner", "Latency", "Source")
	for _, result := range results {
		tb.AddRow(result.Address, scanLabel(result.Zone), result.Port, scanLabel(result.Banner), checkLatencyLabel(result.LatencyMS), scanLabel(result.Source))
	}
	fmt.Fprint(cmdOutput(c), tb.String())
}

func writeScanJSON(c *gcli.Command, results []core.ScanResult) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmdOutput(c), string(data))
	return nil
}

func scanLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

// ScanHostName derives a stable, readable host name from a discovery result.
func ScanHostName(result core.ScanResult) string {
	host := result.Host
	if idx := strings.IndexByte(host, '%'); idx >= 0 {
		host = host[:idx]
	}
	if strings.Contains(host, ":") {
		parts := strings.FieldsFunc(host, func(r rune) bool { return r == ':' })
		if len(parts) > 4 {
			parts = parts[len(parts)-4:]
		}
		return "lan-" + strings.Join(parts, "-")
	}
	return "lan-" + strings.ReplaceAll(host, ".", "-")
}

// saveScanHosts appends discovered hosts without overwriting existing entries
// and without writing any credentials.
func saveScanHosts(results []core.ScanResult, group string) (saved, skipped int, err error) {
	if len(results) == 0 {
		return 0, 0, nil
	}
	config, err := core.LoadConfig()
	if err != nil {
		return 0, 0, err
	}

	taken := make(map[string]struct{}, len(config.Hosts))
	for _, host := range config.Hosts {
		taken[host.Name] = struct{}{}
	}

	for _, result := range results {
		if hasScanHost(config.Hosts, result, group) {
			skipped++
			continue
		}
		host := core.Host{
			Name:  uniqueScanHostName(ScanHostName(result), taken),
			IP:    scanResultHost(result),
			Port:  result.Port,
			Group: group,
		}
		core.NormalizeHostFields(&host)
		taken[host.Name] = struct{}{}
		config.Hosts = append(config.Hosts, host)
		saved++
	}

	if saved == 0 {
		return 0, skipped, nil
	}
	if err := core.SaveConfig(config); err != nil {
		return 0, 0, err
	}
	return saved, skipped, nil
}

func hasScanHost(hosts []core.Host, result core.ScanResult, group string) bool {
	ip := scanResultHost(result)
	for _, host := range hosts {
		if strings.TrimSpace(host.IP) == ip && host.Port == result.Port && core.HostGroupName(host) == group {
			return true
		}
	}
	return false
}

// scanResultHost is the saved IP, keeping an IPv6 zone attached.
func scanResultHost(result core.ScanResult) string {
	if result.Zone != "" && !strings.Contains(result.Host, "%") {
		return result.Host + "%" + result.Zone
	}
	return result.Host
}

func uniqueScanHostName(base string, taken map[string]struct{}) string {
	if _, ok := taken[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, ok := taken[candidate]; !ok {
			return candidate
		}
	}
}

func signalScanContext() (context.Context, context.CancelFunc) {
	return notifyContext(context.Background(), os.Interrupt)
}
