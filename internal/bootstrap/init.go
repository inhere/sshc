package bootstrap

import (
	"github.com/inhere/sshc/internal/command"

	"github.com/gookit/goutil/cflag/capp"
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

func NewApp() *capp.App {
	app := capp.NewWith("sshc", version, "simple ssh host manage and command runner")
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
