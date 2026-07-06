package command

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/gcli/v3"
)

var runRemote = core.ExecuteRemote

func NewRunCmd() *gcli.Command {
	opts := &runFlagOptions{}
	cmd := &gcli.Command{
		Name:    "run",
		Desc:    "run a remote command",
		Aliases: []string{"exec"},
		Help: strings.TrimSpace(`
Examples:
  sshc run devhost -- uptime
  sshc run 192.168.1.10 -- docker ps
  sshc run devhost --script ./deploy.sh
  sshc run devhost --cwd /opt/app -- python -m app
  sshc run inner-db --jump bastion -- hostname
  sshc run devhost --sudo -- apt-get update
  sshc run devhost --sudo-user app --cwd /opt/app -- whoami
  sshc run devhost --timeout 30s --kill-after 5s -- systemctl status nginx
  sshc run devhost -e APP_ENV=prod -e DEBUG=1 -- printenv APP_ENV
  sshc run devhost --efile ./remote.env -- env
  sshc run lxc-app -- hostname
  sshc run devhost --script ./deploy.sh --keep-remote-script
  sshc run devhost --script ./deploy.sh --remote-script-dir /opt/app/tmp

Env file format:
  # comments and blank lines are ignored
  APP_ENV=prod
  export DEBUG=1
  NAME="hello world"

Notes:
  - Remote commands must be placed after --.
  - Use --script for multiline shell, here-doc, source/venv activation, or heavy quoting.
  - Remote timeout requires the remote host to provide the timeout command.
  - Sudo options require passwordless sudo or a root SSH user.
  - Script mode uploads the local file to /tmp by default and runs it with bash.
  - With --script --sudo-user, the uploaded script is readable by local remote users.
  - See docs/deploy-examples.md for common deployment command sequences.
  - Environment variables are injected as a shell prefix, so SSH AcceptEnv is not required.
  - Every run writes a JSON log line under the configured logs_path or ~/.config/sshc/logs.
  - command_proxy hosts support command execution through via hosts, but not --script yet.
`),
		Config: func(c *gcli.Command) {
			c.StrOpt(&opts.Timeout, "timeout", "", "", "command timeout, eg: 30s, 2m, or bare seconds")
			c.StrOpt(&opts.KillAfter, "kill-after", "", "", "force kill delay after timeout, eg: 30s or bare seconds")
			c.VarOpt(&opts.Env, "env", "e", "environment variable k=v, repeatable")
			c.StrOpt(&opts.EnvFile, "env-file", "", "", "load environment variables from file")
			c.StrOpt(&opts.EnvFile, "efile", "", "", "load environment variables from file")
			c.StrOpt(&opts.CWD, "cwd", "", "", "remote working directory")
			c.StrOpt(&opts.Jump, "jump", "", "", "jump host name or ip")
			c.BoolOpt(&opts.Sudo, "sudo", "", false, "run remote command with sudo")
			c.StrOpt(&opts.SudoUser, "sudo-user", "", "", "run remote command as user via sudo")
			c.StrOpt(&opts.Script, "script", "", "", "local shell script to upload and run")
			c.StrOpt(&opts.RemoteScriptDir, "remote-script-dir", "", "", "remote directory for uploaded script")
			c.BoolOpt(&opts.KeepRemoteScript, "keep-remote-script", "", false, "keep uploaded remote script")
			c.AddArg("target", "host ip or name", true)
			c.AddArg("command", "remote command after --", false, true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			target := strings.TrimSpace(c.Arg("target").String())
			command := strings.TrimSpace(strings.Join(remoteCommandArgs(c.Arg("command").Strings()), " "))
			if command == "" && strings.TrimSpace(opts.Script) == "" {
				return errors.New("remote command or --script is required")
			}
			if command != "" && strings.TrimSpace(opts.Script) != "" {
				return errors.New("remote command and --script cannot be used together")
			}

			runOptions, err := buildRunOptions(*opts)
			if err != nil {
				return err
			}
			host, err := resolveCommandHostWithOptions(target, core.ResolveConnectionOptions{Jump: opts.Jump})
			if err != nil {
				return err
			}
			if err := applyHostRunDefaults(&runOptions, host); err != nil {
				return err
			}

			startedAt := core.Now()
			if runOptions.ScriptPath != "" && runOptions.RemoteScriptPath == "" {
				runOptions.RemoteScriptPath = core.NewRemoteScriptPathInDir(startedAt, runOptions.RemoteScriptDir)
			}
			logBackend, logVia, proxiedCommand := commandProxyLogFields(host, command, runOptions)
			out, err := runRemote(host, command, runOptions)
			logErr := core.AppendRunLog(host, core.RunLogRecord{
				Target:           target,
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
			if len(out) > 0 {
				fmt.Fprint(cmdOutput(c), string(out))
			}
			if err != nil {
				writeRunScriptFailureContext(cmdOutput(c), runOptions)
			}
			if err == nil && logErr != nil {
				return logErr
			}
			return err
		},
	}
	return cmd
}

func commandProxyLogFields(host core.Host, command string, opts core.RunOptions) (backend, via, proxiedCommand string) {
	if !core.IsCommandProxyHost(host) {
		return "", "", ""
	}
	backend = core.HostBackendCommandProxy
	via = strings.TrimSpace(host.Via)
	plan, err := core.PlanCommandProxyRun(host, command, opts)
	if err != nil {
		return backend, via, ""
	}
	return backend, core.HostLogName(plan.Via), plan.ProxiedCommand
}

func applyHostRunDefaults(opts *core.RunOptions, host core.Host) error {
	if opts.Timeout <= 0 && strings.TrimSpace(host.RunTimeout) != "" {
		timeout, err := core.ParseTimeout(host.RunTimeout)
		if err != nil {
			return err
		}
		opts.Timeout = timeout
	}
	if strings.TrimSpace(opts.RemoteScriptDir) == "" {
		opts.RemoteScriptDir = strings.TrimSpace(host.RemoteScriptDir)
	}
	return nil
}

func remoteCommandArgs(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}
	return args
}

type runFlagOptions struct {
	Timeout          string
	KillAfter        string
	Env              gcli.Strings
	EnvFile          string
	CWD              string
	Jump             string
	Sudo             bool
	SudoUser         string
	Script           string
	RemoteScriptDir  string
	KeepRemoteScript bool
}

func buildRunOptions(flags runFlagOptions) (core.RunOptions, error) {
	timeout, err := core.ParseTimeout(flags.Timeout)
	if err != nil {
		return core.RunOptions{}, err
	}
	killAfter, err := core.ParseTimeout(flags.KillAfter)
	if err != nil {
		return core.RunOptions{}, err
	}
	env, err := core.LoadRunEnv(flags.EnvFile, flags.Env.Strings())
	if err != nil {
		return core.RunOptions{}, err
	}
	sudoUser := strings.TrimSpace(flags.SudoUser)
	if flags.Sudo && sudoUser != "" {
		return core.RunOptions{}, errors.New("--sudo and --sudo-user cannot be used together")
	}
	if sudoUser != "" {
		if err := core.ValidateSudoUser(sudoUser); err != nil {
			return core.RunOptions{}, err
		}
	}
	scriptPath := strings.TrimSpace(flags.Script)
	remoteScriptDir := strings.TrimSpace(flags.RemoteScriptDir)
	if remoteScriptDir != "" && scriptPath == "" {
		return core.RunOptions{}, errors.New("--remote-script-dir requires --script")
	}
	return core.RunOptions{
		Timeout:          timeout,
		KillAfter:        killAfter,
		Env:              env,
		CWD:              strings.TrimSpace(flags.CWD),
		Sudo:             flags.Sudo,
		SudoUser:         sudoUser,
		ScriptPath:       scriptPath,
		RemoteScriptDir:  remoteScriptDir,
		KeepRemoteScript: flags.KeepRemoteScript,
	}, nil
}

func writeRunScriptFailureContext(out io.Writer, opts core.RunOptions) {
	if strings.TrimSpace(opts.ScriptPath) == "" {
		return
	}
	fmt.Fprintf(out, "sshc: local_script=%s\n", opts.ScriptPath)
	if strings.TrimSpace(opts.RemoteScriptPath) != "" {
		fmt.Fprintf(out, "sshc: remote_script=%s\n", opts.RemoteScriptPath)
	}
	if strings.TrimSpace(opts.SudoUser) != "" {
		fmt.Fprintf(out, "sshc: sudo_user=%s\n", opts.SudoUser)
	} else if opts.Sudo {
		fmt.Fprintln(out, "sshc: sudo=true")
	}
	if !opts.KeepRemoteScript {
		fmt.Fprintln(out, "sshc: use --keep-remote-script to inspect the uploaded script")
	}
}
