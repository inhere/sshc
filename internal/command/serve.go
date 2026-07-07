package command

import (
	"context"
	"os"
	"os/signal"
	"fmt"
	"strings"
	"time"

	"github.com/gookit/gcli/v3"
	"github.com/gookit/goutil/sysutil"
	"github.com/inhere/sshc/internal/server"
)

var openBrowser = sysutil.OpenBrowser

func NewServeCmd() *gcli.Command {
	opts := struct {
		Addr     string
		NoOpen   bool
		Readonly bool
		WebDir   string
		Token    string
	}{}
	return &gcli.Command{
		Name: "serve",
		Desc: "start local web console",
		Help: strings.TrimSpace(`
Examples:
  sshc serve
  sshc serve --addr 127.0.0.1:8822
  sshc serve --addr 127.0.0.1:0 --no-open
  sshc serve --readonly
  sshc serve --web-dir ./web/dist
  sshc serve --addr 0.0.0.0:8822 --token random
  sshc serve --addr 0.0.0.0:8822 --token "change-me"

Notes:
  - serve opens the browser by default; use --no-open to disable it.
  - :0 is normalized to 127.0.0.1:0 to avoid exposing the console on all interfaces.
  - Binding a non-loopback address requires --token.
  - Use --token random to generate a one-time access token.
`),
		Config: func(c *gcli.Command) {
			c.StrOpt(&opts.Addr, "addr", "", server.DefaultAddr, "HTTP listen address")
			c.BoolOpt(&opts.NoOpen, "no-open", "", false, "do not open browser after start")
			c.BoolOpt(&opts.Readonly, "readonly", "", false, "disable write operations and terminal sessions")
			c.StrOpt(&opts.WebDir, "web-dir", "", "", "serve web assets from directory")
			c.StrOpt(&opts.Token, "token", "", "", "access token for non-loopback listen address")
		},
		Func: func(c *gcli.Command, _ []string) error {
			token := strings.TrimSpace(opts.Token)
			generatedToken := ""
			if strings.EqualFold(token, "random") {
				var err error
				generatedToken, err = server.GenerateToken()
				if err != nil {
					return err
				}
				token = generatedToken
			}
			srv, err := server.New(server.Config{
				Addr:     opts.Addr,
				Open:     !opts.NoOpen,
				Readonly: opts.Readonly,
				WebDir:   opts.WebDir,
				Token:    token,
			})
			if err != nil {
				return err
			}
			ctx, stop := signalContext()
			defer stop()
			url, err := srv.Start(ctx)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "sshc serve listening on %s\n", url)
			if generatedToken != "" {
				fmt.Fprintf(cmdOutput(c), "sshc serve token: %s\n", generatedToken)
			}
			if srv.Config().Open {
				if err := openBrowser(url); err != nil {
					fmt.Fprintf(cmdOutput(c), "warning: open browser failed: %v\n", err)
				}
			}
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return srv.Shutdown(shutdownCtx)
		},
	}
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}
