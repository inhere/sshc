package command

import (
	"errors"
	"fmt"
	"strings"

	"sshc/internal/core"

	"github.com/gookit/goutil/cflag"
	"github.com/gookit/goutil/cflag/capp"
)

var runRemote = core.ExecuteRemote

func NewRunCmd() *capp.Cmd {
	opts := &runFlagOptions{}
	cmd := capp.NewCmd("run", "run a remote command", func(c *capp.Cmd) error {
		target := strings.TrimSpace(c.Arg("target").String())
		command := strings.TrimSpace(strings.Join(c.Arg("command").Strings(), " "))
		if command == "" && strings.TrimSpace(opts.Script) == "" {
			return errors.New("remote command or --script is required")
		}
		if command != "" && strings.TrimSpace(opts.Script) != "" {
			return errors.New("remote command and --script cannot be used together")
		}

		store, err := core.LoadStore()
		if err != nil {
			return err
		}
		host, ok, err := store.ResolveHost(target)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("host %q not found", target)
		}

		runOptions, err := buildRunOptions(*opts)
		if err != nil {
			return err
		}

		startedAt := core.Now()
		if runOptions.ScriptPath != "" && runOptions.RemoteScriptPath == "" {
			runOptions.RemoteScriptPath = core.NewRemoteScriptPath(startedAt)
		}
		out, err := runRemote(host, command, runOptions)
		logErr := core.AppendRunLog(host, core.RunLogRecord{
			Target:           target,
			Command:          command,
			Status:           core.RunStatus(err),
			StartedAt:        startedAt,
			DurationMS:       core.SinceMS(startedAt),
			Output:           string(out),
			Error:            core.ErrorString(err),
			Script:           runOptions.ScriptPath,
			RemoteScript:     runOptions.RemoteScriptPath,
			KeepRemoteScript: runOptions.KeepRemoteScript,
		})
		if len(out) > 0 {
			fmt.Fprint(c.Output(), string(out))
		}
		if err == nil && logErr != nil {
			return logErr
		}
		return err
	})
	cmd.Aliases = []string{"exec"}
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc run devhost -- uptime
  sshc run 192.168.1.10 -- docker ps
  sshc run devhost --script ./deploy.sh
  sshc run devhost --timeout 30s -- systemctl status nginx
  sshc run devhost -e APP_ENV=prod -e DEBUG=1 -- printenv APP_ENV
  sshc run devhost --efile ./remote.env -- env
  sshc run devhost --script ./deploy.sh --keep-remote-script

Options:
  --timeout accepts Go duration values like 500ms, 30s, 2m.
  --timeout also accepts bare seconds, for example 5 means 5s.
  -e/--env can be repeated. Later values override env-file values.
  --env-file/--efile loads a single env file with KEY=value lines.
  --script uploads a local shell script to /tmp and runs it with bash.
  --keep-remote-script keeps the uploaded script for debugging.

Env file format:
  # comments and blank lines are ignored
  APP_ENV=prod
  export DEBUG=1
  NAME="hello world"

Notes:
  - Remote commands must be placed after --.
  - Use --script instead of command after -- for multi-line deployment scripts.
  - Environment variables are injected as a shell prefix, so SSH AcceptEnv is not required.
  - Every run writes a JSON log line under ~/.config/sshc/logs/<host>.log.
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&opts.Timeout, "timeout", "", "command timeout, eg: 30s, 2m, or bare seconds")
		c.Var(&opts.Env, "env", "environment variable k=v, repeatable;;e")
		c.StringVar(&opts.EnvFile, "env-file", "", "load environment variables from file;;efile")
		c.StringVar(&opts.Script, "script", "", "local shell script to upload and run")
		c.BoolVar(&opts.KeepRemoteScript, "keep-remote-script", false, "keep uploaded remote script")
		c.AddArg("target", "host ip or name", true)
		c.AddArg("command", "remote command after --", false, nil, true)
	}
	return cmd
}

type runFlagOptions struct {
	Timeout          string
	Env              cflag.Strings
	EnvFile          string
	Script           string
	KeepRemoteScript bool
}

func buildRunOptions(flags runFlagOptions) (core.RunOptions, error) {
	timeout, err := core.ParseTimeout(flags.Timeout)
	if err != nil {
		return core.RunOptions{}, err
	}
	env, err := core.LoadRunEnv(flags.EnvFile, flags.Env.Strings())
	if err != nil {
		return core.RunOptions{}, err
	}
	return core.RunOptions{
		Timeout:          timeout,
		Env:              env,
		ScriptPath:       strings.TrimSpace(flags.Script),
		KeepRemoteScript: flags.KeepRemoteScript,
	}, nil
}
