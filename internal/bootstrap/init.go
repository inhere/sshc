package bootstrap

import (
	"github.com/inhere/sshc/internal/command"

	"github.com/gookit/gcli/v3"
)

var (
	version   string
	gitHash   string
	buildTime string
)

// type BuildInfo struct {
// 	Version   string
// 	GitHash   string
// 	BuildTime string
// }

// SetBuildInfo sets the build information for the application.
func SetBuildInfo(versionStr, gitHashStr, buildTimeStr string) {
	version = versionStr
	gitHash = gitHashStr
	buildTime = buildTimeStr
}

func NewApp() *gcli.App {
	app := gcli.NewApp()
	app.Name = "sshc"
	app.Desc = "simple ssh host manage and command runner"
	if version != "" {
		app.Version = version
	}
	app.Add(
		command.NewAddCmd(),
		command.NewRunCmd(),
		command.NewUploadCmd(),
		command.NewDownloadCmd(),
		command.NewListCmd(),
		command.NewLogCmd(),
		command.NewLoginCmd(),
	)
	return app
}
