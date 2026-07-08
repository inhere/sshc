package core

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const runtimeLogDirName = "runtime"

type RuntimeLogger struct {
	file *os.File
}

func InitRuntimeLogger() (*RuntimeLogger, error) {
	path, err := RuntimeLogPath()
	if err != nil {
		setDiscardLogger()
		return &RuntimeLogger{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		setDiscardLogger()
		return &RuntimeLogger{}, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		setDiscardLogger()
		return &RuntimeLogger{}, err
	}
	logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{
		ReplaceAttr: replaceLogTimeAttr,
	}))
	slog.SetDefault(logger)
	return &RuntimeLogger{file: file}, nil
}

func RuntimeLogPath() (string, error) {
	dir, err := runtimeLogsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, runtimeLogDirName, "sshc.log"), nil
}

func runtimeLogsDir() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	path, _, err := readRuntimeLogsPath()
	if err != nil {
		return "", err
	}
	if path != "" {
		path = expandUserPath(path)
		if filepath.IsAbs(path) {
			return path, nil
		}
		return filepath.Join(root, path), nil
	}
	return filepath.Join(root, logDirName), nil
}

func readRuntimeLogsPath() (string, string, error) {
	path, data, err := readConfigFile()
	if err != nil {
		return "", "", err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return "", path, nil
	}
	var config struct {
		LogsPath string `json:"logs_path"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", path, err
	}
	return strings.TrimSpace(config.LogsPath), path, nil
}

func (l *RuntimeLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

func RuntimeLogArgs(args []string) []string {
	out := make([]string, 0, len(args))
	maskNext := false
	for _, arg := range args {
		if maskNext {
			out = append(out, "***")
			maskNext = false
			continue
		}
		if name, value, ok := splitFlagValue(arg); ok {
			if isSensitiveRuntimeFlag(name) {
				out = append(out, valueWithMaskedFlag(arg, value))
				continue
			}
		}
		if isSensitiveRuntimeFlag(flagName(arg)) {
			out = append(out, arg)
			maskNext = true
			continue
		}
		if isJoinedShortSensitiveFlag(arg) {
			out = append(out, arg[:2]+"***")
			continue
		}
		out = append(out, arg)
	}
	return out
}

func RuntimeCommandName(args []string) string {
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		return strings.TrimSpace(arg)
	}
	return ""
}

func RuntimeDurationMS(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

func setDiscardLogger() {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func splitFlagValue(arg string) (name string, value string, ok bool) {
	if !strings.HasPrefix(arg, "-") {
		return "", "", false
	}
	idx := strings.Index(arg, "=")
	if idx < 0 {
		return "", "", false
	}
	return flagName(arg[:idx]), arg[idx+1:], true
}

func valueWithMaskedFlag(arg, value string) string {
	return strings.TrimSuffix(arg, value) + "***"
}

func flagName(arg string) string {
	arg = strings.TrimLeft(strings.TrimSpace(arg), "-")
	return strings.ToLower(strings.TrimSpace(arg))
}

func isSensitiveRuntimeFlag(name string) bool {
	switch name {
	case "p", "password", "pwd", "pass", "token", "key", "secret", "e", "env":
		return true
	default:
		return false
	}
}

func isJoinedShortSensitiveFlag(arg string) bool {
	if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") || len(arg) <= 2 {
		return false
	}
	return isSensitiveRuntimeFlag(arg[1:2])
}
