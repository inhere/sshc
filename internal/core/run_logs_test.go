package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunLogDirUsesConfiguredLogsPath(t *testing.T) {
	path := withTempConfig(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"logs_path":"runtime/logs"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	dir, err := runLogDir()
	if err != nil {
		t.Fatal(err)
	}
	home, err := userHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "sshc", "runtime", "logs")
	if dir != want {
		t.Fatalf("log dir = %q, want %q", dir, want)
	}
}

func TestRunLogDirExpandsConfiguredLogsPath(t *testing.T) {
	path := withTempConfig(t)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"logs_path":"~/sshc-logs"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	dir, err := runLogDir()
	if err != nil {
		t.Fatal(err)
	}
	home, err := userHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "sshc-logs")
	if dir != want {
		t.Fatalf("log dir = %q, want %q", dir, want)
	}
}

func TestReadRunLogsMatchesAndTails(t *testing.T) {
	withTempConfig(t)
	host := Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}

	records := []RunLogRecord{
		{Target: "devhost", Command: "echo alpha", Status: "success"},
		{Target: "devhost", Command: "echo beta", Status: "success"},
		{Target: "devhost", Command: "echo gamma", Status: "success"},
	}
	for _, rec := range records {
		if err := AppendRunLog(host, rec); err != nil {
			t.Fatalf("append log: %v", err)
		}
	}

	matched, err := ReadRunLogs("devhost", "beta", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 || !strings.Contains(matched[0], "echo beta") {
		t.Fatalf("matched logs = %#v", matched)
	}

	tailed, err := ReadRunLogs("devhost", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(tailed) != 2 || !strings.Contains(tailed[0], "echo beta") || !strings.Contains(tailed[1], "echo gamma") {
		t.Fatalf("tailed logs = %#v", tailed)
	}
}

func TestAppendRunLogWritesTaskID(t *testing.T) {
	withTempConfig(t)
	host := Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}

	if err := AppendRunLog(host, RunLogRecord{Target: "devhost", Command: "echo ok", Status: "success"}); err != nil {
		t.Fatalf("append log: %v", err)
	}
	lines, err := ReadRunLogs("devhost", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], `"task_id":"`) {
		t.Fatalf("log line missing task_id: %#v", lines)
	}
}

func TestAppendRunLogKeepsSmallOutputInline(t *testing.T) {
	withTempConfig(t)
	host := Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}

	if err := AppendRunLog(host, RunLogRecord{TaskID: "task-small", Target: "devhost", Command: "echo ok", Status: "success", Output: "ok\n"}); err != nil {
		t.Fatalf("append log: %v", err)
	}
	lines, err := ReadRunLogs("devhost", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	line := lines[0]
	for _, want := range []string{`"task_id":"task-small"`, `"output":"ok\n"`, `"output_inline":true`} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q does not contain %q", line, want)
		}
	}
	if strings.Contains(line, `"output_file"`) {
		t.Fatalf("small output should not use output_file: %q", line)
	}
}

func TestAppendRunLogStoresLargeOutputFile(t *testing.T) {
	withTempConfig(t)
	startedAt := time.Date(2026, 7, 4, 17, 30, 12, 0, time.Local)
	host := Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
	output := strings.Repeat("a", maxInlineRunOutputBytes+1)

	if err := AppendRunLog(host, RunLogRecord{TaskID: "task-large", Target: "devhost", Command: "cat big", Status: "success", StartedAt: startedAt, Output: output}); err != nil {
		t.Fatalf("append log: %v", err)
	}
	lines, err := ReadRunLogs("devhost", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	line := lines[0]
	for _, want := range []string{`"task_id":"task-large"`, `"output_file":"20260704/task-large.out.log"`, `"output_inline":false`, `"output_preview":"`} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q does not contain %q", line, want)
		}
	}
	if strings.Contains(line, `"output":"`) {
		t.Fatalf("large output should not be inline: %q", line)
	}
	dir, err := runLogDir()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "20260704", "task-large.out.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != output {
		t.Fatalf("output file content mismatch")
	}
}

func TestAppendRunLogLargeOutputStoresShortPreview(t *testing.T) {
	withTempConfig(t)
	host := Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
	output := strings.Repeat("好", 600)

	if err := AppendRunLog(host, RunLogRecord{TaskID: "task-preview", Target: "devhost", Command: "echo big", Status: "success", Output: output}); err != nil {
		t.Fatalf("append log: %v", err)
	}
	lines, err := ReadRunLogs("devhost", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatal(err)
	}
	preview, ok := record["output_preview"].(string)
	if !ok || preview == "" {
		t.Fatalf("missing output_preview in %v", record)
	}
	if len(preview) > maxRunOutputPreviewBytes || !strings.HasPrefix(output, preview) {
		t.Fatalf("bad preview len=%d value=%q", len(preview), preview)
	}
}

func TestReadRunLogOutputByIDFromInlineOutput(t *testing.T) {
	withTempConfig(t)
	host := Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
	if err := AppendRunLog(host, RunLogRecord{TaskID: "task-inline", Target: "devhost", Command: "echo ok", Status: "success", Output: "ok\n"}); err != nil {
		t.Fatalf("append log: %v", err)
	}

	data, err := ReadRunLogOutputByID("devhost", "task-inline")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok\n" {
		t.Fatalf("output = %q", string(data))
	}
}

func TestReadRunLogOutputByIDFromOutputFile(t *testing.T) {
	withTempConfig(t)
	host := Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
	output := strings.Repeat("line\n", 300)
	if err := AppendRunLog(host, RunLogRecord{TaskID: "task-file", Target: "devhost", Command: "cat big", Status: "success", Output: output}); err != nil {
		t.Fatalf("append log: %v", err)
	}

	data, err := ReadRunLogOutputByID("devhost", "task-file")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != output {
		t.Fatalf("output file = %q", string(data))
	}
}

func TestReadRunLogOutputByIDReturnsErrorWhenMissing(t *testing.T) {
	withTempConfig(t)
	if _, err := ReadRunLogOutputByID("devhost", "missing"); err == nil {
		t.Fatal("expected missing task error")
	}
}

func TestReadRunLogsSelectedLinesRange(t *testing.T) {
	withTempConfig(t)
	host := Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
	for _, command := range []string{"one", "two", "three"} {
		if err := AppendRunLog(host, RunLogRecord{Target: "devhost", Command: command, Status: "success"}); err != nil {
			t.Fatalf("append log: %v", err)
		}
	}

	lines, err := ReadRunLogsSelected("devhost", "", 200, "2,3")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || !strings.Contains(lines[0], `"command":"two"`) || !strings.Contains(lines[1], `"command":"three"`) {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestParseLogLineRange(t *testing.T) {
	start, end, ok, err := ParseLogLineRange("2,5")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || start != 2 || end != 5 {
		t.Fatalf("range = %d,%d ok=%v", start, end, ok)
	}
	if _, _, _, err := ParseLogLineRange("5,2"); err == nil {
		t.Fatal("expected invalid range error")
	}
}

func TestRunLogTimeFormatUsesMillisecondsWithoutZone(t *testing.T) {
	withTempConfig(t)
	loc := time.FixedZone("CST", 8*60*60)
	fixed := time.Date(2026, 7, 3, 17, 16, 14, 350724100, loc)
	t.Cleanup(SetNowForTest(func() time.Time { return fixed }))

	host := Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
	if err := AppendRunLog(host, RunLogRecord{
		Target:    "devhost",
		Command:   "echo ok",
		Status:    "success",
		StartedAt: fixed,
	}); err != nil {
		t.Fatalf("append log: %v", err)
	}

	lines, err := ReadRunLogs("devhost", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("log lines len = %d, want 1", len(lines))
	}
	line := lines[0]
	for _, want := range []string{`"time":"2026-07-03T17:16:14.350"`, `"started_at":"2026-07-03T17:16:14.350"`} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q does not contain %q", line, want)
		}
	}
	if strings.Contains(line, "+08") || strings.Contains(line, "3507241") {
		t.Fatalf("log line has unwanted zone or sub-millisecond precision: %q", line)
	}
}
