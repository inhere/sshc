package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/inhere/sshc/internal/core"
)

func Run(args []string) int {
	start := time.Now()
	runtimeLogger, _ := core.InitRuntimeLogger()
	defer runtimeLogger.Close()

	slog.Info("command_start",
		slog.String("command", core.RuntimeCommandName(args)),
		slog.Any("args", core.RuntimeLogArgs(args)),
		slog.Int("pid", os.Getpid()),
		slog.String("cwd", mustGetwd()),
		slog.String("version", version),
		slog.String("git_commit", gitHash),
	)

	code := NewApp().Run(args)
	attrs := []slog.Attr{
		slog.String("command", core.RuntimeCommandName(args)),
		slog.Int("exit_code", code),
		slog.Int64("duration_ms", core.RuntimeDurationMS(start)),
	}
	if code != 0 {
		slog.LogAttrs(context.Background(), slog.LevelError, "command_end", attrs...)
		return code
	}
	slog.LogAttrs(context.Background(), slog.LevelInfo, "command_end", attrs...)
	return code
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}
