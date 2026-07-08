package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/inhere/sshc/internal/bootstrap"
	"github.com/inhere/sshc/internal/core"
)

// Build metadata, injected by Makefile LDFLAGS (-X main.Version etc.).
var (
	Version   string
	GitCommit string
	BuildDate string
)

func main() {
	bootstrap.SetBuildInfo(Version, GitCommit, BuildDate)

	args := os.Args[1:]
	start := time.Now()
	runtimeLogger, _ := core.InitRuntimeLogger()
	defer runtimeLogger.Close()
	slog.Info("command_start",
		slog.String("command", core.RuntimeCommandName(args)),
		slog.Any("args", core.RuntimeLogArgs(args)),
		slog.Int("pid", os.Getpid()),
		slog.String("cwd", mustGetwd()),
		slog.String("version", Version),
		slog.String("git_commit", GitCommit),
	)

	code := bootstrap.NewApp().Run(args)
	attrs := []slog.Attr{
		slog.String("command", core.RuntimeCommandName(args)),
		slog.Int("exit_code", code),
		slog.Int64("duration_ms", core.RuntimeDurationMS(start)),
	}
	if code != 0 {
		slog.LogAttrs(context.Background(), slog.LevelError, "command_end", attrs...)
		_ = runtimeLogger.Close()
		// ccolor.Fprintf(os.Stderr, "<err>ERROR:</> exit code %d\n", code)
		os.Exit(code)
	}
	slog.LogAttrs(context.Background(), slog.LevelInfo, "command_end", attrs...)
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}
