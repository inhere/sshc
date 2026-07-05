package main

import (
	"os"

	"github.com/inhere/sshc/internal/bootstrap"
)

// Build metadata, injected by Makefile LDFLAGS (-X main.Version etc.).
var (
	Version   string
	GitCommit string
	BuildDate string
)

func main() {
	bootstrap.SetBuildInfo(Version, GitCommit, BuildDate)

	if code := bootstrap.NewApp().Run(os.Args[1:]); code != 0 {
		// ccolor.Fprintf(os.Stderr, "<err>ERROR:</> exit code %d\n", code)
		os.Exit(code)
	}
}
