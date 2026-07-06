package command

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/gcli/v3"
)

type batchRunFlagOptions struct {
	Hosts          string
	HostsFile      string
	Group          string
	AuthRef        string
	User           string
	PasswordPrompt bool
	KeyPath        string
	Port           int
	Parallel       int
	FailFast       bool
	Run            runFlagOptions
}

func NewBatchRunCmd() *gcli.Command {
	opts := &batchRunFlagOptions{}
	cmd := &gcli.Command{
		Name:    "batch-run",
		Desc:    "run a command or script on multiple hosts",
		Aliases: []string{"brun"},
		Help: strings.TrimSpace(`
Examples:
  sshc batch-run --hosts devhost,web-2 -- uptime
  sshc brun --hosts devhost,web-2 -- uptime
  sshc batch-run --group testing --script ./deploy.sh
  sshc batch-run --hosts-file hosts.txt -- hostname
  sshc batch-run --hosts-file ips.txt --auth dev-root --script ./init.sh
  sshc batch-run --hosts devhost,lxc-app -- uptime
  sshc batch-run --group testing --parallel 5 --fail-fast -- uptime
  sshc batch-run --hosts-file ips.txt --auth dev-root -u root --port 22 --script ./init.sh

Host sources:
  - Exactly one of --hosts, --hosts-file, or --group is required.
  - Saved hosts are resolved first.
  - Unresolved IP/hostname targets can be used with shared auth options.

Notes:
  - Remote commands must be placed after --.
  - Use --script for multiline shell, here-doc, source/venv activation, or heavy quoting.
  - Saved command_proxy hosts can be mixed with normal SSH hosts for command mode.
  - Every target writes a JSON log line with task_id under the configured logs_path.
`),
		Config: func(c *gcli.Command) {
			c.StrOpt(&opts.Hosts, "hosts", "", "", "comma-separated host names or IPs")
			c.StrOpt(&opts.HostsFile, "hosts-file", "", "", "file with one host target per line")
			c.StrOpt(&opts.Group, "group", "", "", "host group name")
			c.StrOpt(&opts.AuthRef, "auth", "", "", "auth profile for raw host targets")
			c.StrOpt(&opts.User, "user", "u", "", "ssh username for raw host targets")
			c.BoolOpt(&opts.PasswordPrompt, "password", "p", false, "prompt for shared hidden password")
			c.StrOpt(&opts.KeyPath, "key", "", "", "ssh private key path for raw host targets")
			c.IntOpt(&opts.Port, "port", "", 0, "ssh port for raw host targets")
			c.IntOpt(&opts.Parallel, "parallel", "", 3, "max hosts to run at once")
			c.BoolOpt(&opts.FailFast, "fail-fast", "", false, "stop starting new hosts after first failure")
			c.StrOpt(&opts.Run.Timeout, "timeout", "", "", "command timeout, eg: 30s, 2m, or bare seconds")
			c.StrOpt(&opts.Run.KillAfter, "kill-after", "", "", "force kill delay after timeout, eg: 30s or bare seconds")
			c.VarOpt(&opts.Run.Env, "env", "e", "environment variable k=v, repeatable")
			c.StrOpt(&opts.Run.EnvFile, "env-file", "", "", "load environment variables from file")
			c.StrOpt(&opts.Run.EnvFile, "efile", "", "", "load environment variables from file")
			c.StrOpt(&opts.Run.CWD, "cwd", "", "", "remote working directory")
			c.BoolOpt(&opts.Run.Sudo, "sudo", "", false, "run remote command with sudo")
			c.StrOpt(&opts.Run.SudoUser, "sudo-user", "", "", "run remote command as user via sudo")
			c.StrOpt(&opts.Run.Script, "script", "", "", "local shell script to upload and run")
			c.StrOpt(&opts.Run.RemoteScriptDir, "remote-script-dir", "", "", "remote directory for uploaded script")
			c.BoolOpt(&opts.Run.KeepRemoteScript, "keep-remote-script", "", false, "keep uploaded remote script")
			c.AddArg("command", "remote command after --", false, true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			command := strings.TrimSpace(strings.Join(remoteCommandArgs(c.Arg("command").Strings()), " "))
			if command == "" && strings.TrimSpace(opts.Run.Script) == "" {
				return errors.New("remote command or --script is required")
			}
			if command != "" && strings.TrimSpace(opts.Run.Script) != "" {
				return errors.New("remote command and --script cannot be used together")
			}

			runOptions, err := buildRunOptions(opts.Run)
			if err != nil {
				return err
			}
			source := buildBatchHostSource(*opts)
			if opts.PasswordPrompt {
				source.Overrides.Password = strings.TrimSpace(readInteractivePassword("Password: "))
			}
			hosts, err := core.ResolveBatchHosts(source)
			if err != nil {
				return err
			}
			if opts.Parallel < 1 {
				return errors.New("--parallel must be greater than 0")
			}
			return runBatch(c, hosts, command, runOptions, opts.Parallel, opts.FailFast)
		},
	}
	return cmd
}

func buildBatchHostSource(opts batchRunFlagOptions) core.BatchHostSource {
	overrides := core.HostOverrides{
		User:    strings.TrimSpace(opts.User),
		KeyPath: strings.TrimSpace(opts.KeyPath),
		Port:    opts.Port,
	}
	return core.BatchHostSource{
		Hosts:     batchHostsList(opts.Hosts),
		HostsFile: strings.TrimSpace(opts.HostsFile),
		Group:     strings.TrimSpace(opts.Group),
		Overrides: overrides,
		AuthRef:   strings.TrimSpace(opts.AuthRef),
		AllowRaw:  batchAllowsRaw(opts),
	}
}

func batchHostsList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return []string{value}
}

func batchAllowsRaw(opts batchRunFlagOptions) bool {
	return strings.TrimSpace(opts.AuthRef) != "" ||
		strings.TrimSpace(opts.User) != "" ||
		strings.TrimSpace(opts.KeyPath) != "" ||
		opts.PasswordPrompt
}

func runBatch(c *gcli.Command, hosts []core.Host, command string, baseOptions core.RunOptions, parallel int, failFast bool) error {
	startedBatch := time.Now()
	results := make([]batchRunResult, 0, len(hosts))
	if parallel > len(hosts) {
		parallel = len(hosts)
	}
	resultCh := make(chan batchRunResult, parallel)
	started := 0
	running := 0
	next := 0
	stopped := false
	startHost := func(host core.Host) {
		started++
		running++
		go func() {
			resultCh <- runBatchHost(host, command, baseOptions)
		}()
	}
	for next < len(hosts) && running < parallel {
		startHost(hosts[next])
		next++
	}
	for running > 0 {
		result := <-resultCh
		running--
		results = append(results, result)
		writeBatchRunBlock(c, result)
		if result.Error != nil && failFast {
			stopped = true
		}
		for !stopped && next < len(hosts) && running < parallel {
			startHost(hosts[next])
			next++
		}
	}
	skipped := len(hosts) - started
	writeBatchRunSummary(c, results, skipped, time.Since(startedBatch))
	if failed := failedBatchHosts(results); len(failed) > 0 {
		return fmt.Errorf("batch-run failed on: %s", strings.Join(failed, ", "))
	}
	return nil
}

type batchRunResult struct {
	Host       core.Host
	Target     string
	Output     string
	Error      error
	Status     string
	DurationMS int64
}

func runBatchHost(host core.Host, command string, baseOptions core.RunOptions) batchRunResult {
	runOptions := baseOptions
	startedAt := core.Now()
	if err := applyHostRunDefaults(&runOptions, host); err != nil {
		return batchRunResult{
			Host:       host,
			Target:     core.HostLogName(host),
			Error:      err,
			Status:     core.RunStatus(err),
			DurationMS: core.SinceMS(startedAt),
		}
	}
	if runOptions.ScriptPath != "" && runOptions.RemoteScriptPath == "" {
		runOptions.RemoteScriptPath = core.NewRemoteScriptPathInDir(startedAt, runOptions.RemoteScriptDir)
	}
	logBackend, logVia, proxiedCommand := commandProxyLogFields(host, command, runOptions)
	out, err := runRemote(host, command, runOptions)
	logErr := core.AppendRunLog(host, core.RunLogRecord{
		Target:           core.HostLogName(host),
		Command:          command,
		Status:           core.RunStatus(err),
		StartedAt:        startedAt,
		DurationMS:       core.SinceMS(startedAt),
		Output:           string(out),
		Error:            core.ErrorString(err),
		CWD:              runOptions.CWD,
		Backend:          logBackend,
		Via:              logVia,
		ProxiedCommand:   proxiedCommand,
		Script:           runOptions.ScriptPath,
		RemoteScript:     runOptions.RemoteScriptPath,
		KeepRemoteScript: runOptions.KeepRemoteScript,
	})
	if err == nil && logErr != nil {
		err = logErr
	}
	return batchRunResult{
		Host:       host,
		Target:     core.HostLogName(host),
		Output:     string(out),
		Error:      err,
		Status:     core.RunStatus(err),
		DurationMS: core.SinceMS(startedAt),
	}
}

func writeBatchRunBlock(c *gcli.Command, result batchRunResult) {
	if core.IsCommandProxyHost(result.Host) {
		fmt.Fprintf(cmdOutput(c), "==> %s (command_proxy via:%s)\n", core.HostLogName(result.Host), strings.TrimSpace(result.Host.Via))
	} else {
		fmt.Fprintf(cmdOutput(c), "==> %s (%s@%s:%d)\n", core.HostLogName(result.Host), result.Host.User, result.Host.IP, result.Host.Port)
	}
	if result.Output != "" {
		fmt.Fprint(cmdOutput(c), result.Output)
		if !strings.HasSuffix(result.Output, "\n") {
			fmt.Fprintln(cmdOutput(c))
		}
	}
	if result.Error != nil {
		fmt.Fprintf(cmdOutput(c), "sshc: error: %v\n", result.Error)
	}
	fmt.Fprintln(cmdOutput(c))
}

func writeBatchRunSummary(c *gcli.Command, results []batchRunResult, skipped int, elapsed time.Duration) {
	success, failed := 0, 0
	for _, result := range results {
		if result.Error != nil {
			failed++
			continue
		}
		success++
	}
	fmt.Fprintf(cmdOutput(c), "Summary: total=%d success=%d failed=%d skipped=%d elapsed=%s\n", len(results)+skipped, success, failed, skipped, formatElapsed(elapsed))
	if failed > 0 {
		fmt.Fprintf(cmdOutput(c), "Failed hosts: %s\n", strings.Join(failedBatchHosts(results), ", "))
	}
}

func failedBatchHosts(results []batchRunResult) []string {
	var failed []string
	for _, result := range results {
		if result.Error != nil {
			failed = append(failed, result.Target)
		}
	}
	return failed
}
