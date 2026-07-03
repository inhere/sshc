package bootstrap

import (
	"sshc/internal/command"

	"github.com/gookit/goutil/cflag/capp"
)

const version = "0.1.0"

func NewApp() *capp.App {
	app := capp.NewWith("sshc", version, "simple ssh command runner")
	app.Add(
		command.NewAddCmd(),
		command.NewRunCmd(),
		command.NewSCPCmd(),
		command.NewDownloadCmd(),
		command.NewListCmd(),
		command.NewLogCmd(),
	)
	return app
}
