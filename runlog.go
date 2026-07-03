package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const logDirName = "logs"

var now = time.Now

type RunLogRecord struct {
	Target     string
	Command    string
	Status     string
	StartedAt  time.Time
	DurationMS int64
	Output     string
	Error      string
}

func appendRunLog(host Host, rec RunLogRecord) error {
	path, err := runLogPath(host)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{}))
	attrs := []slog.Attr{
		slog.String("target", rec.Target),
		slog.String("host", hostLogName(host)),
		slog.String("ip", host.IP),
		slog.String("user", host.User),
		slog.Int("port", host.Port),
		slog.String("command", rec.Command),
		slog.String("status", rec.Status),
		slog.String("started_at", rec.StartedAt.Format(time.RFC3339Nano)),
		slog.Int64("duration_ms", rec.DurationMS),
	}
	if rec.Output != "" {
		attrs = append(attrs, slog.String("output", rec.Output))
	}
	if rec.Error != "" {
		attrs = append(attrs, slog.String("error", rec.Error))
	}

	logger.LogAttrs(context.Background(), slog.LevelInfo, "run", attrs...)
	return nil
}

func readRunLogs(target, match string, tail int) ([]string, error) {
	if tail < 1 {
		return nil, errors.New("tail must be greater than 0")
	}

	files, err := runLogFiles(target)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}

	var lines []string
	for _, file := range files {
		readLines, err := readMatchingLines(file, match)
		if err != nil {
			return nil, err
		}
		lines = append(lines, readLines...)
	}
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return lines, nil
}

func runLogFiles(target string) ([]string, error) {
	dir, err := runLogDir()
	if err != nil {
		return nil, err
	}
	if target != "" {
		return []string{filepath.Join(dir, safeLogName(target)+".log")}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func readMatchingLines(path, match string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var lines []string
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			line = strings.TrimRight(line, "\r\n")
			if match == "" || strings.Contains(line, match) {
				lines = append(lines, line)
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		return nil, err
	}
	return lines, nil
}

func runLogPath(host Host) (string, error) {
	dir, err := runLogDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, safeLogName(hostLogName(host))+".log"), nil
}

func runLogDir() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, logDirName), nil
}

func configRoot() (string, error) {
	dir, err := userHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".config", "sshc"), nil
}

func hostLogName(host Host) string {
	if strings.TrimSpace(host.Name) != "" {
		return strings.TrimSpace(host.Name)
	}
	return strings.TrimSpace(host.IP)
}

func runStatus(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func sinceMS(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

var unsafeLogNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeLogName(name string) string {
	name = strings.TrimSpace(name)
	name = unsafeLogNameChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._-")
	if name == "" {
		return "host"
	}
	return name
}
