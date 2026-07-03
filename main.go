package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

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

	runRemote = executeRemote
)

func main() {
	if err := newApp().RunWithArgs(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func newApp() *capp.App {
	app := capp.NewWith("sshc", version, "simple ssh command runner")
	app.Add(newAddCmd(), newRunCmd(), newListCmd())
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

		out, err := runRemote(host, command)
		if len(out) > 0 {
			fmt.Fprint(c.Output(), string(out))
		}
		return err
	})
	cmd.OnAdd = func(c *capp.Cmd) {
		c.AddArg("target", "host ip or name", true)
		c.AddArg("command", "remote command after --", true, nil, true)
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
