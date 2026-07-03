package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gookit/goutil/cflag"
	"github.com/gookit/goutil/cflag/capp"
)

const version = "0.1.0"

var (
	addOpts = struct {
		IP       string
		Name     string
		User     string
		Password string
		Port     int
	}{Port: defaultSSHPort}

	runRemote      = executeRemote
	scpUpload      = uploadRemote
	downloadRemote = fetchRemote
)

func main() {
	if err := newApp().RunWithArgs(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func newApp() *capp.App {
	app := capp.NewWith("sshc", version, "simple ssh command runner")
	app.Add(newAddCmd(), newRunCmd(), newSCPCmd(), newDownloadCmd(), newListCmd(), newLogCmd())
	return app
}

func newAddCmd() *capp.Cmd {
	cmd := capp.NewCmd("add", "add or update an ssh host", func(c *capp.Cmd) error {
		if addOpts.Port == 0 {
			addOpts.Port = defaultSSHPort
		}

		host := Host{
			Name:     strings.TrimSpace(addOpts.Name),
			IP:       strings.TrimSpace(addOpts.IP),
			User:     strings.TrimSpace(addOpts.User),
			Password: addOpts.Password,
			Port:     addOpts.Port,
		}
		if host.Name == "" {
			host.Name = host.IP
		}

		store, err := loadStore()
		if err != nil {
			return err
		}
		if err := store.Upsert(host); err != nil {
			return err
		}
		if err := saveStore(store); err != nil {
			return err
		}

		fmt.Fprintf(c.Output(), "saved %s (%s:%d)\n", host.Name, host.IP, host.Port)
		return nil
	})
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&addOpts.IP, "ip", "", "ssh host ip or hostname;true")
		c.StringVar(&addOpts.Name, "name", "", "host alias")
		c.StringVar(&addOpts.User, "user", "", "ssh username;true;u")
		c.StringVar(&addOpts.Password, "password", "", "ssh password;true;p")
		c.IntVar(&addOpts.Port, "port", defaultSSHPort, "ssh port")
	}
	return cmd
}

func newRunCmd() *capp.Cmd {
	opts := &runFlagOptions{}
	cmd := capp.NewCmd("run", "run a remote command", func(c *capp.Cmd) error {
		target := strings.TrimSpace(c.Arg("target").String())
		command := strings.TrimSpace(strings.Join(c.Arg("command").Strings(), " "))
		if command == "" {
			return errors.New("remote command is required")
		}

		store, err := loadStore()
		if err != nil {
			return err
		}
		host, ok := store.Find(target)
		if !ok {
			return fmt.Errorf("host %q not found", target)
		}

		runOptions, err := buildRunOptions(*opts)
		if err != nil {
			return err
		}

		startedAt := now()
		out, err := runRemote(host, command, runOptions)
		logErr := appendRunLog(host, RunLogRecord{
			Target:     target,
			Command:    command,
			Status:     runStatus(err),
			StartedAt:  startedAt,
			DurationMS: sinceMS(startedAt),
			Output:     string(out),
			Error:      errorString(err),
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
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&opts.Timeout, "timeout", "", "command timeout, eg: 30s, 2m, or bare seconds")
		c.Var(&opts.Env, "env", "environment variable k=v, repeatable;;e")
		c.StringVar(&opts.EnvFile, "env-file", "", "load environment variables from file;;efile")
		c.AddArg("target", "host ip or name", true)
		c.AddArg("command", "remote command after --", true, nil, true)
	}
	return cmd
}

type runFlagOptions struct {
	Timeout string
	Env     cflag.Strings
	EnvFile string
}

type RunOptions struct {
	Timeout time.Duration
	Env     map[string]string
}

func buildRunOptions(flags runFlagOptions) (RunOptions, error) {
	timeout, err := parseTimeout(flags.Timeout)
	if err != nil {
		return RunOptions{}, err
	}
	env, err := loadRunEnv(flags.EnvFile, flags.Env.Strings())
	if err != nil {
		return RunOptions{}, err
	}
	return RunOptions{Timeout: timeout, Env: env}, nil
}

var scpOpts = struct {
	LocalPath  string
	RemotePath string
}{}

func newSCPCmd() *capp.Cmd {
	cmd := capp.NewCmd("scp", "upload a file or directory to remote host", func(c *capp.Cmd) error {
		target := strings.TrimSpace(c.Arg("target").String())
		localPath := strings.TrimSpace(scpOpts.LocalPath)
		remotePath := strings.TrimSpace(scpOpts.RemotePath)
		if localPath == "" {
			return errors.New("local path is required")
		}
		if remotePath == "" {
			return errors.New("remote path is required")
		}

		store, err := loadStore()
		if err != nil {
			return err
		}
		host, ok := store.Find(target)
		if !ok {
			return fmt.Errorf("host %q not found", target)
		}

		if err := scpUpload(host, localPath, remotePath); err != nil {
			return err
		}
		fmt.Fprintf(c.Output(), "uploaded %s to %s:%s\n", localPath, hostLogName(host), remotePath)
		return nil
	})
	cmd.Aliases = []string{"upload"}
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&scpOpts.LocalPath, "local", "", "local file or directory path;true;l")
		c.StringVar(&scpOpts.RemotePath, "remote", "", "remote file or directory path;true;r")
		c.AddArg("target", "host ip or name", true)
	}
	return cmd
}

var downloadOpts = struct {
	LocalPath  string
	RemotePath string
}{}

func newDownloadCmd() *capp.Cmd {
	cmd := capp.NewCmd("download", "download a file or directory from remote host", func(c *capp.Cmd) error {
		target := strings.TrimSpace(c.Arg("target").String())
		localPath := strings.TrimSpace(downloadOpts.LocalPath)
		remotePath := strings.TrimSpace(downloadOpts.RemotePath)
		if localPath == "" {
			return errors.New("local path is required")
		}
		if remotePath == "" {
			return errors.New("remote path is required")
		}

		store, err := loadStore()
		if err != nil {
			return err
		}
		host, ok := store.Find(target)
		if !ok {
			return fmt.Errorf("host %q not found", target)
		}

		if err := downloadRemote(host, remotePath, localPath); err != nil {
			return err
		}
		fmt.Fprintf(c.Output(), "downloaded %s:%s to %s\n", hostLogName(host), remotePath, localPath)
		return nil
	})
	cmd.Aliases = []string{"dl"}
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&downloadOpts.LocalPath, "local", "", "local destination path;true;l")
		c.StringVar(&downloadOpts.RemotePath, "remote", "", "remote file or directory path;true;r")
		c.AddArg("target", "host ip or name", true)
	}
	return cmd
}

func newListCmd() *capp.Cmd {
	return capp.NewCmd("list", "list saved ssh hosts", func(c *capp.Cmd) error {
		store, err := loadStore()
		if err != nil {
			return err
		}
		for _, host := range store.Hosts {
			name := host.Name
			if name == "" {
				name = host.IP
			}
			fmt.Fprintf(c.Output(), "%s\t%s@%s:%d\n", name, host.User, host.IP, host.Port)
		}
		return nil
	}).WithConfigFn(func(c *capp.Cmd) {
		c.Aliases = []string{"ls"}
	})
}

var logOpts = struct {
	Match string
	Tail  int
}{Tail: 200}

func newLogCmd() *capp.Cmd {
	cmd := capp.NewCmd("log", "show or search run logs", func(c *capp.Cmd) error {
		target := strings.TrimSpace(c.Arg("target").String())
		logTarget, err := resolveLogTarget(target)
		if err != nil {
			return err
		}
		lines, err := readRunLogs(logTarget, logOpts.Match, logOpts.Tail)
		if err != nil {
			return err
		}
		for _, line := range lines {
			fmt.Fprintln(c.Output(), line)
		}
		return nil
	})
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&logOpts.Match, "match", "", "match log lines by keyword;;m")
		c.IntVar(&logOpts.Tail, "tail", 200, "max lines to print")
		c.AddArg("target", "host ip or name, empty means all logs", false)
	}
	return cmd
}

func resolveLogTarget(target string) (string, error) {
	if target == "" {
		return "", nil
	}
	store, err := loadStore()
	if err != nil {
		return "", err
	}
	if host, ok := store.Find(target); ok {
		return hostLogName(host), nil
	}
	return target, nil
}
