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
	RerunFailed    string
	Summary        string
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
  sshc batch-run --rerun-failed 20260708-120102-a1b2

Host sources:
  - Exactly one of --hosts, --hosts-file, or --group is required.
  - Saved hosts are resolved first.
  - Unresolved IP/hostname targets can be used with shared auth options.

Notes:
  - Remote commands must be placed after --.
  - Use --script for multiline shell, here-doc, source/venv activation, or heavy quoting.
  - Saved command_proxy hosts can be mixed with normal SSH hosts for command mode.
  - Every target writes a JSON log line with task_id under the configured logs_path.
  - Every batch writes a summary JSONL record under logs_path/batch.
`),
		Config: func(c *gcli.Command) {
			c.StrOpt(&opts.Hosts, "hosts", "", "", "comma-separated host names or IPs")
			c.StrOpt(&opts.HostsFile, "hosts-file", "", "", "file with one host target per line")
			c.StrOpt(&opts.Group, "group", "", "", "host group name")
			c.StrOpt(&opts.RerunFailed, "rerun-failed", "", "", "rerun failed hosts from a previous batch_id")
			c.StrOpt(&opts.Summary, "summary", "", "table", "summary output mode: table")
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
			if err := validateBatchSummaryMode(opts.Summary); err != nil {
				return err
			}
			if strings.TrimSpace(opts.RerunFailed) != "" {
				return rerunFailedBatch(c, *opts, command)
			}
			if err := validateNormalBatchRun(*opts, command); err != nil {
				return err
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
			return runBatch(c, batchRunRequest{
				Hosts:      hosts,
				Command:    command,
				RunOptions: runOptions,
				RunFlags:   opts.Run,
				Source:     buildBatchLogSource(*opts),
				Parallel:   opts.Parallel,
				FailFast:   opts.FailFast,
				Summary:    opts.Summary,
			})
		},
	}
	return cmd
}

func validateBatchSummaryMode(mode string) error {
	mode = strings.TrimSpace(mode)
	if mode == "" || mode == "table" {
		return nil
	}
	return fmt.Errorf("--summary %s is not supported", mode)
}

func validateNormalBatchRun(opts batchRunFlagOptions, command string) error {
	if command == "" && strings.TrimSpace(opts.Run.Script) == "" {
		return errors.New("remote command or --script is required")
	}
	if command != "" && strings.TrimSpace(opts.Run.Script) != "" {
		return errors.New("remote command and --script cannot be used together")
	}
	return nil
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

type batchRunRequest struct {
	Hosts      []core.Host
	Command    string
	RunOptions core.RunOptions
	RunFlags   runFlagOptions
	Source     core.BatchRunSourceLog
	Parallel   int
	FailFast   bool
	Summary    string
	RerunOf    string
}

func runBatch(c *gcli.Command, req batchRunRequest) error {
	startedBatch := core.Now()
	startedWall := time.Now()
	batchID := core.NewBatchID(startedBatch)
	results := make([]batchRunResult, 0, len(req.Hosts))
	parallel := req.Parallel
	if parallel > len(req.Hosts) {
		parallel = len(req.Hosts)
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
			resultCh <- runBatchHost(host, req.Command, req.RunOptions)
		}()
	}
	for next < len(req.Hosts) && running < parallel {
		startHost(req.Hosts[next])
		next++
	}
	for running > 0 {
		result := <-resultCh
		running--
		results = append(results, result)
		writeBatchRunBlock(c, result)
		if result.Error != nil && req.FailFast {
			stopped = true
		}
		for !stopped && next < len(req.Hosts) && running < parallel {
			startHost(req.Hosts[next])
			next++
		}
	}
	skipped := len(req.Hosts) - started
	elapsed := time.Since(startedWall)
	record := buildBatchRunRecord(batchID, req, results, skipped, startedBatch, core.Now())
	logErr := core.AppendBatchRunLog(record)
	writeBatchRunSummary(c, batchID, results, skipped, elapsed)
	if failed := failedBatchHosts(results); len(failed) > 0 {
		if logErr != nil {
			fmt.Fprintf(cmdOutput(c), "sshc: warning: write batch summary: %v\n", logErr)
		}
		return fmt.Errorf("batch-run failed on: %s", strings.Join(failed, ", "))
	}
	return logErr
}

func rerunFailedBatch(c *gcli.Command, opts batchRunFlagOptions, command string) error {
	return errors.New("--rerun-failed is not implemented yet")
}

type batchRunResult struct {
	Host       core.Host
	Target     string
	Output     string
	Error      error
	Status     string
	TaskID     string
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
	taskID := core.NewRunTaskID(startedAt, host, core.RunLogRecord{Command: command, Script: runOptions.ScriptPath})
	logErr := core.AppendRunLog(host, core.RunLogRecord{
		Target:           core.HostLogName(host),
		TaskID:           taskID,
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
		TaskID:     taskID,
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

func writeBatchRunSummary(c *gcli.Command, batchID string, results []batchRunResult, skipped int, elapsed time.Duration) {
	success, failed := 0, 0
	for _, result := range results {
		if result.Error != nil {
			failed++
			continue
		}
		success++
	}
	fmt.Fprintf(cmdOutput(c), "Batch ID: %s\n", batchID)
	fmt.Fprintf(cmdOutput(c), "Summary: total=%d success=%d failed=%d skipped=%d elapsed=%s\n", len(results)+skipped, success, failed, skipped, formatElapsed(elapsed))
	if failed > 0 {
		fmt.Fprintf(cmdOutput(c), "Failed hosts: %s\n", strings.Join(failedBatchHosts(results), ", "))
	}
}

func buildBatchRunRecord(batchID string, req batchRunRequest, results []batchRunResult, skipped int, startedAt, endedAt time.Time) core.BatchRunRecord {
	record := core.BatchRunRecord{
		BatchID:      batchID,
		RerunOf:      strings.TrimSpace(req.RerunOf),
		StartedAt:    formatBatchLogTime(startedAt),
		EndedAt:      formatBatchLogTime(endedAt),
		Source:       req.Source,
		Command:      req.Command,
		Script:       strings.TrimSpace(req.RunOptions.ScriptPath),
		TaskName:     "",
		Options:      buildBatchRunLogOptions(req.RunFlags, req.RunOptions),
		Hosts:        hostLogNames(req.Hosts),
		SkippedCount: skipped,
		Results:      make([]core.BatchRunResult, 0, len(results)),
	}
	for _, result := range results {
		if result.Error != nil {
			record.FailedCount++
		} else {
			record.SuccessCount++
		}
		record.Results = append(record.Results, core.BatchRunResult{
			Host:       result.Target,
			Status:     result.Status,
			TaskID:     result.TaskID,
			DurationMS: result.DurationMS,
			Error:      core.ErrorString(result.Error),
		})
	}
	return record
}

func buildBatchLogSource(opts batchRunFlagOptions) core.BatchRunSourceLog {
	source := core.BatchRunSourceLog{
		AuthRef:  strings.TrimSpace(opts.AuthRef),
		User:     strings.TrimSpace(opts.User),
		KeyPath:  strings.TrimSpace(opts.KeyPath),
		Port:     opts.Port,
		AllowRaw: batchAllowsRaw(opts),
	}
	switch {
	case strings.TrimSpace(opts.Hosts) != "":
		source.Kind = "hosts"
		source.Value = strings.TrimSpace(opts.Hosts)
	case strings.TrimSpace(opts.HostsFile) != "":
		source.Kind = "hosts_file"
		source.Value = strings.TrimSpace(opts.HostsFile)
	case strings.TrimSpace(opts.Group) != "":
		source.Kind = "group"
		source.Value = strings.TrimSpace(opts.Group)
	}
	return source
}

func buildBatchRunLogOptions(flags runFlagOptions, opts core.RunOptions) core.BatchRunOptions {
	return core.BatchRunOptions{
		Timeout:          durationLogString(opts.Timeout),
		KillAfter:        durationLogString(opts.KillAfter),
		Env:              core.MaskRunEnv(opts.Env),
		EnvFile:          strings.TrimSpace(flags.EnvFile),
		CWD:              strings.TrimSpace(opts.CWD),
		Sudo:             opts.Sudo,
		SudoUser:         strings.TrimSpace(opts.SudoUser),
		ScriptPath:       strings.TrimSpace(opts.ScriptPath),
		RemoteScriptDir:  strings.TrimSpace(opts.RemoteScriptDir),
		KeepRemoteScript: opts.KeepRemoteScript,
	}
}

func durationLogString(value time.Duration) string {
	if value <= 0 {
		return ""
	}
	return value.String()
}

func formatBatchLogTime(value time.Time) string {
	if value.IsZero() {
		value = core.Now()
	}
	return value.Format("2006-01-02T15:04:05.000")
}

func hostLogNames(hosts []core.Host) []string {
	names := make([]string, 0, len(hosts))
	for _, host := range hosts {
		names = append(names, core.HostLogName(host))
	}
	return names
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
