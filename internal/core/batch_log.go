package core

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const batchLogDirName = "batch"

type BatchRunRecord struct {
	BatchID      string            `json:"batch_id"`
	RerunOf      string            `json:"rerun_of,omitempty"`
	StartedAt    string            `json:"started_at"`
	EndedAt      string            `json:"ended_at"`
	Source       BatchRunSourceLog `json:"source"`
	Command      string            `json:"command,omitempty"`
	Script       string            `json:"script,omitempty"`
	TaskName     string            `json:"task_name,omitempty"`
	Options      BatchRunOptions   `json:"options,omitempty"`
	Hosts        []string          `json:"hosts"`
	SuccessCount int               `json:"success_count"`
	FailedCount  int               `json:"failed_count"`
	SkippedCount int               `json:"skipped_count"`
	Results      []BatchRunResult  `json:"results"`
}

type BatchRunSourceLog struct {
	Kind     string `json:"kind"`
	Value    string `json:"value,omitempty"`
	AuthRef  string `json:"auth_ref,omitempty"`
	User     string `json:"user,omitempty"`
	KeyPath  string `json:"key_path,omitempty"`
	Port     int    `json:"port,omitempty"`
	AllowRaw bool   `json:"allow_raw,omitempty"`
}

type BatchRunOptions struct {
	Timeout          string            `json:"timeout,omitempty"`
	KillAfter        string            `json:"kill_after,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	EnvFile          string            `json:"env_file,omitempty"`
	CWD              string            `json:"cwd,omitempty"`
	Sudo             bool              `json:"sudo,omitempty"`
	SudoUser         string            `json:"sudo_user,omitempty"`
	ScriptPath       string            `json:"script_path,omitempty"`
	RemoteScriptDir  string            `json:"remote_script_dir,omitempty"`
	KeepRemoteScript bool              `json:"keep_remote_script,omitempty"`
}

type BatchRunResult struct {
	Host       string `json:"host"`
	Status     string `json:"status"`
	TaskID     string `json:"task_id,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

func NewBatchID(startedAt time.Time) string {
	if startedAt.IsZero() {
		startedAt = now()
	}
	random := make([]byte, 2)
	if _, err := rand.Read(random); err != nil {
		sum := startedAt.UnixNano()
		random[0] = byte(sum)
		random[1] = byte(sum >> 8)
	}
	return startedAt.Format(runTaskTimeLayout) + "-" + hex.EncodeToString(random)
}

func AppendBatchRunLog(record BatchRunRecord) error {
	if strings.TrimSpace(record.BatchID) == "" {
		return errors.New("batch_id is required")
	}
	when := now()
	if record.StartedAt != "" {
		if parsed, err := time.Parse(logTimeLayout, record.StartedAt); err == nil {
			when = parsed
		}
	}
	path, err := BatchLogPath(when)
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

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func BatchLogPath(when time.Time) (string, error) {
	dir, err := LogsDir()
	if err != nil {
		return "", err
	}
	if when.IsZero() {
		when = now()
	}
	return filepath.Join(dir, batchLogDirName, when.Format(runOutputDateLayout)+".jsonl"), nil
}

func ReadBatchRunByID(batchID string) (BatchRunRecord, bool, error) {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return BatchRunRecord{}, false, errors.New("batch_id is required")
	}
	files, err := batchLogFiles()
	if err != nil {
		return BatchRunRecord{}, false, err
	}
	for i := len(files) - 1; i >= 0; i-- {
		record, ok, err := findBatchRunRecord(files[i], batchID)
		if err != nil {
			return BatchRunRecord{}, false, err
		}
		if ok {
			return record, true, nil
		}
	}
	return BatchRunRecord{}, false, nil
}

func FailedBatchRunHosts(record BatchRunRecord) []string {
	hosts := make([]string, 0, record.FailedCount)
	for _, result := range record.Results {
		if result.Status == "success" {
			continue
		}
		if host := strings.TrimSpace(result.Host); host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}

func MaskRunEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	masked := make(map[string]string, len(env))
	for key, value := range env {
		if IsSensitiveEnvName(key) {
			masked[key] = MaskedSecret
			continue
		}
		masked[key] = value
	}
	return masked
}

func HasMaskedRunEnv(env map[string]string) bool {
	for _, value := range env {
		if value == MaskedSecret {
			return true
		}
	}
	return false
}

func IsSensitiveEnvName(name string) bool {
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	for _, token := range []string{"PASSWORD", "PASS", "PWD", "TOKEN", "SECRET", "KEY", "PRIVATE", "CREDENTIAL", "AUTH"} {
		if strings.Contains(name, token) {
			return true
		}
	}
	return false
}

func batchLogFiles() ([]string, error) {
	dir, err := LogsDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, batchLogDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func findBatchRunRecord(path, batchID string) (BatchRunRecord, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return BatchRunRecord{}, false, nil
		}
		return BatchRunRecord{}, false, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			line = strings.TrimRight(line, "\r\n")
			var record BatchRunRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				return BatchRunRecord{}, false, fmt.Errorf("%s: %w", path, err)
			}
			if record.BatchID == batchID {
				return record, true, nil
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		return BatchRunRecord{}, false, err
	}
	return BatchRunRecord{}, false, nil
}
