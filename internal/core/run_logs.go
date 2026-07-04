package core

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const logDirName = "logs"

var now = time.Now

const (
	logTimeLayout            = "2006-01-02T15:04:05.000"
	runTaskTimeLayout        = "20060102-150405"
	runOutputDateLayout      = "20060102"
	maxInlineRunOutputBytes  = 1024
	maxRunOutputPreviewBytes = 512
)

type RunLogRecord struct {
	Target           string
	TaskID           string
	Command          string
	Status           string
	StartedAt        time.Time
	DurationMS       int64
	Output           string
	OutputFile       string
	OutputBytes      int64
	OutputSHA256     string
	OutputInline     bool
	OutputPreview    string
	Error            string
	CWD              string
	Script           string
	RemoteScript     string
	KeepRemoteScript bool
}

func Now() time.Time {
	return now()
}

func SetNowForTest(fn func() time.Time) func() {
	old := now
	now = fn
	return func() { now = old }
}

func AppendRunLog(host Host, rec RunLogRecord) error {
	logTime := rec.StartedAt
	if logTime.IsZero() {
		logTime = now()
	}
	rec.StartedAt = logTime
	if strings.TrimSpace(rec.TaskID) == "" {
		rec.TaskID = NewRunTaskID(logTime, host, rec)
	} else {
		rec.TaskID = safeLogName(rec.TaskID)
	}
	if err := prepareRunLogOutput(&rec, logTime); err != nil {
		return err
	}

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

	handler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		ReplaceAttr: replaceLogTimeAttr,
	})
	attrs := []slog.Attr{
		slog.String("target", rec.Target),
		slog.String("task_id", rec.TaskID),
		slog.String("host", HostLogName(host)),
		slog.String("ip", host.IP),
		slog.String("user", host.User),
		slog.Int("port", host.Port),
		slog.String("command", rec.Command),
		slog.String("status", rec.Status),
		slog.String("started_at", formatLogTime(rec.StartedAt)),
		slog.Int64("duration_ms", rec.DurationMS),
		slog.Int64("output_bytes", rec.OutputBytes),
		slog.Bool("output_inline", rec.OutputInline),
	}
	if rec.Output != "" {
		attrs = append(attrs, slog.String("output", rec.Output))
	}
	if rec.OutputPreview != "" {
		attrs = append(attrs, slog.String("output_preview", rec.OutputPreview))
	}
	if rec.OutputFile != "" {
		attrs = append(attrs, slog.String("output_file", rec.OutputFile))
	}
	if rec.OutputSHA256 != "" {
		attrs = append(attrs, slog.String("output_sha256", rec.OutputSHA256))
	}
	if rec.Error != "" {
		attrs = append(attrs, slog.String("error", rec.Error))
	}
	if rec.CWD != "" {
		attrs = append(attrs, slog.String("cwd", rec.CWD))
	}
	if rec.Script != "" {
		attrs = append(attrs,
			slog.String("script", rec.Script),
			slog.String("remote_script", rec.RemoteScript),
			slog.Bool("keep_remote_script", rec.KeepRemoteScript),
		)
	}

	record := slog.NewRecord(logTime, slog.LevelInfo, "run", 0)
	record.AddAttrs(attrs...)
	return handler.Handle(context.Background(), record)
}

func NewRunTaskID(startedAt time.Time, host Host, rec RunLogRecord) string {
	if startedAt.IsZero() {
		startedAt = now()
	}
	seed := fmt.Sprintf("%s|%s|%s|%d|%s|%s|%d|",
		startedAt.Format(time.RFC3339Nano),
		HostLogName(host),
		host.IP,
		host.Port,
		rec.Command,
		rec.Script,
		now().UnixNano(),
	)
	random := make([]byte, 8)
	if _, err := rand.Read(random); err == nil {
		seed += hex.EncodeToString(random)
	}
	sum := sha256.Sum256([]byte(seed))
	return startedAt.Format(runTaskTimeLayout) + "-" + hex.EncodeToString(sum[:])[:8]
}

func prepareRunLogOutput(rec *RunLogRecord, startedAt time.Time) error {
	output := rec.Output
	rec.OutputBytes = int64(len(output))
	if output == "" {
		rec.OutputInline = false
		return nil
	}

	sum := sha256.Sum256([]byte(output))
	rec.OutputSHA256 = hex.EncodeToString(sum[:])
	if len(output) <= maxInlineRunOutputBytes {
		rec.OutputInline = true
		return nil
	}

	absPath, relPath, err := RunOutputPath(rec.TaskID, startedAt)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(output); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	rec.Output = ""
	rec.OutputInline = false
	rec.OutputFile = relPath
	rec.OutputPreview = safeUTF8Prefix(output, maxRunOutputPreviewBytes)
	return nil
}

func RunOutputPath(taskID string, startedAt time.Time) (string, string, error) {
	if strings.TrimSpace(taskID) == "" {
		return "", "", errors.New("task_id is required")
	}
	taskID = safeLogName(taskID)
	if startedAt.IsZero() {
		startedAt = now()
	}
	dir, err := runLogDir()
	if err != nil {
		return "", "", err
	}
	dateDir := startedAt.Format(runOutputDateLayout)
	relPath := filepath.ToSlash(filepath.Join(dateDir, taskID+".out.log"))
	return filepath.Join(dir, filepath.FromSlash(relPath)), relPath, nil
}

func safeUTF8Prefix(value string, limit int) string {
	if limit < 1 || len(value) <= limit {
		return value
	}
	cut := limit
	for cut > 0 && !utf8.ValidString(value[:cut]) {
		cut--
	}
	return value[:cut]
}

func replaceLogTimeAttr(groups []string, attr slog.Attr) slog.Attr {
	if attr.Key == slog.TimeKey {
		if value, ok := attr.Value.Any().(time.Time); ok {
			attr.Value = slog.StringValue(formatLogTime(value))
		}
	}
	return attr
}

func formatLogTime(value time.Time) string {
	return value.Format(logTimeLayout)
}

func ReadRunLogs(target, match string, tail int) ([]string, error) {
	return ReadRunLogsSelected(target, match, tail, "")
}

func ReadRunLogsSelected(target, match string, tail int, rangeSpec string) ([]string, error) {
	if strings.TrimSpace(rangeSpec) != "" {
		tail = 0
	} else if tail < 1 {
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
	return SelectLogLines(lines, rangeSpec, tail)
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

func ReadRunLogOutputByID(target, taskID string) ([]byte, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errors.New("task_id is required")
	}
	files, err := runLogFiles(target)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		record, ok, err := findRunLogRecordByID(file, taskID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if record.OutputInline {
			return []byte(record.Output), nil
		}
		if strings.TrimSpace(record.OutputFile) == "" {
			return nil, fmt.Errorf("task %s has no output", taskID)
		}
		dir, err := runLogDir()
		if err != nil {
			return nil, err
		}
		path := record.OutputFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, filepath.FromSlash(path))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read output for task %s: %w", taskID, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("task %s not found", taskID)
}

type runLogJSONRecord struct {
	TaskID       string `json:"task_id"`
	Output       string `json:"output"`
	OutputFile   string `json:"output_file"`
	OutputInline bool   `json:"output_inline"`
}

func findRunLogRecordByID(path, taskID string) (runLogJSONRecord, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return runLogJSONRecord{}, false, nil
		}
		return runLogJSONRecord{}, false, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			line = strings.TrimRight(line, "\r\n")
			var record runLogJSONRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				return runLogJSONRecord{}, false, err
			}
			if record.TaskID == taskID {
				return record, true, nil
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		return runLogJSONRecord{}, false, err
	}
	return runLogJSONRecord{}, false, nil
}

func SelectLogLines(lines []string, rangeSpec string, tail int) ([]string, error) {
	start, end, ok, err := ParseLogLineRange(rangeSpec)
	if err != nil {
		return nil, err
	}
	if ok {
		if len(lines) == 0 || start > len(lines) {
			return nil, nil
		}
		if end > len(lines) {
			end = len(lines)
		}
		return lines[start-1 : end], nil
	}
	if tail > 0 && len(lines) > tail {
		return lines[len(lines)-tail:], nil
	}
	return lines, nil
}

func ParseLogLineRange(spec string) (start int, end int, ok bool, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, 0, false, nil
	}
	parts := strings.Split(spec, ",")
	if len(parts) != 2 {
		return 0, 0, false, fmt.Errorf("invalid lines range %q, want start,end", spec)
	}
	start, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || start < 1 {
		return 0, 0, false, fmt.Errorf("invalid lines start %q", strings.TrimSpace(parts[0]))
	}
	end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || end < 1 {
		return 0, 0, false, fmt.Errorf("invalid lines end %q", strings.TrimSpace(parts[1]))
	}
	if start > end {
		return 0, 0, false, fmt.Errorf("invalid lines range %q: start must be <= end", spec)
	}
	return start, end, true, nil
}

func runLogPath(host Host) (string, error) {
	dir, err := runLogDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, safeLogName(HostLogName(host))+".log"), nil
}

func runLogDir() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	config, err := LoadConfigSettings()
	if err != nil {
		return "", err
	}
	if path := strings.TrimSpace(config.LogsPath); path != "" {
		path = expandUserPath(path)
		if filepath.IsAbs(path) {
			return path, nil
		}
		return filepath.Join(root, path), nil
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

func HostLogName(host Host) string {
	if strings.TrimSpace(host.Name) != "" {
		return strings.TrimSpace(host.Name)
	}
	return strings.TrimSpace(host.IP)
}

func RunStatus(err error) string {
	if err != nil {
		return "error"
	}
	return "success"
}

func ErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func SinceMS(start time.Time) int64 {
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
