package main

import (
	"os"

	"github.com/gookit/goutil/x/ccolor"
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

	if code := bootstrap.NewApp().Run(normalizeArgs(os.Args[1:])); code != 0 {
		ccolor.Fprintf(os.Stderr, "<err>ERROR:</> exit code %d\n", code)
		os.Exit(code)
	}
}

func normalizeArgs(args []string) []string {
	if len(args) >= 2 && args[0] == "--gen-completion" {
		normalized := append([]string{"--gen-completion=" + args[1]}, args[2:]...)
		return normalized
	}
	return args
}
