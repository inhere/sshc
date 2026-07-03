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
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc add --ip 192.168.1.10 -u root -p password
  sshc add --ip 192.168.1.10 --name devhost -u root -p password --port 2222

Notes:
  - If --name is empty, the IP is used as the host name.
  - Adding the same name or IP updates the saved host.
  - Hosts are stored in ~/.config/sshc/hosts.json by default.
  - Passwords are currently stored in plain text. Keep the config file private.
`)
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
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc run devhost -- uptime
  sshc run 192.168.1.10 -- docker ps
  sshc run devhost --timeout 30s -- systemctl status nginx
  sshc run devhost -e APP_ENV=prod -e DEBUG=1 -- printenv APP_ENV
  sshc run devhost --efile ./remote.env -- env

Options:
  --timeout accepts Go duration values like 500ms, 30s, 2m.
  --timeout also accepts bare seconds, for example 5 means 5s.
  -e/--env can be repeated. Later values override env-file values.
  --env-file/--efile loads a single env file with KEY=value lines.

Env file format:
  # comments and blank lines are ignored
  APP_ENV=prod
  export DEBUG=1
  NAME="hello world"

Notes:
  - Remote commands must be placed after --.
  - Environment variables are injected as a shell prefix, so SSH AcceptEnv is not required.
  - Every run writes a JSON log line under ~/.config/sshc/logs/<host>.log.
`)
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
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost
  sshc scp -l ./local-dir -r /tmp/remote-dir devhost
  sshc upload -l ./dist -r /opt/app/dist devhost

Path rules:
  - -l/--local can be a file or directory.
  - -r/--remote is the remote destination path.
  - File upload creates remote parent directories when needed.
  - If remote path ends with / for file upload, the local file name is appended.
  - Directory upload recursively creates directories and files under the remote path.
`)
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
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc download -r /tmp/remote-file.txt -l ./local-file.txt devhost
  sshc download -r /tmp/remote-file.txt -l ./downloads/ devhost
  sshc dl -r /tmp/remote-dir -l ./local-dir devhost

Path rules:
  - -r/--remote can be a remote file or directory.
  - -l/--local is the local destination path.
  - If local path exists as a directory, the remote base name is appended.
  - If local path ends with / or \, the remote base name is appended.
  - Directory download recursively creates local directories and files.
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&downloadOpts.LocalPath, "local", "", "local destination path;true;l")
		c.StringVar(&downloadOpts.RemotePath, "remote", "", "remote file or directory path;true;r")
		c.AddArg("target", "host ip or name", true)
	}
	return cmd
}

func newListCmd() *capp.Cmd {
	cmd := capp.NewCmd("list", "list saved ssh hosts", func(c *capp.Cmd) error {
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
	})
	cmd.Aliases = []string{"ls"}
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc list
  sshc ls

Output:
  name    user@ip:port

Notes:
  - Hosts are read from ~/.config/sshc/hosts.json by default.
  - Set SSHC_CONFIG to use a different hosts file.
`)
	return cmd
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
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc log
  sshc log devhost
  sshc log 192.168.1.10
  sshc log devhost --match uptime
  sshc log devhost -m error --tail 50

Log files:
  ~/.config/sshc/logs/<host>.log

Notes:
  - Without target, all host log files are read in file-name order.
  - With target, sshc resolves a saved host first, so IP can map to the host name log.
  - --match filters raw JSON log lines by substring.
  - --tail limits the final number of printed lines after filtering.
`)
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
