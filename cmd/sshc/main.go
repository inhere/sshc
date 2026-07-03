package main

import (
	"fmt"
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

	if err := bootstrap.NewApp().RunWithArgs(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}
