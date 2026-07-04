package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/inhere/sshc/internal/core"
)

func TestLogCommandShowsOutputByIDWithTail(t *testing.T) {
	withTempConfig(t)
	host := core.Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
	if err := core.AppendRunLog(host, core.RunLogRecord{
		TaskID:  "task-tail",
		Target:  "devhost",
		Command: "printf",
		Status:  "success",
		Output:  "one\ntwo\nthree\n",
	}); err != nil {
		t.Fatalf("append log: %v", err)
	}

	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	app := newTestApp()
	if err := app.RunWithArgs([]string{"log", "devhost", "--id", "task-tail", "--tail", "2"}); err != nil {
		t.Fatalf("log --id --tail: %v", err)
	}
	if got := out.String(); got != "two\nthree\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestLogCommandShowsOutputByIDWithLines(t *testing.T) {
	withTempConfig(t)
	host := core.Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
	if err := core.AppendRunLog(host, core.RunLogRecord{
		TaskID:  "task-lines",
		Target:  "devhost",
		Command: "printf",
		Status:  "success",
		Output:  "one\ntwo\nthree\n",
	}); err != nil {
		t.Fatalf("append log: %v", err)
	}

	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	app := newTestApp()
	if err := app.RunWithArgs([]string{"log", "devhost", "--id", "task-lines", "--lines", "2,2"}); err != nil {
		t.Fatalf("log --id --lines: %v", err)
	}
	if got := out.String(); got != "two\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestLogCommandRejectsIDWithMatch(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	if err := app.RunWithArgs([]string{"log", "--id", "task-id", "--match", "error"}); err == nil {
		t.Fatal("expected --id and --match error")
	}
}

func TestLogCommandRejectsTailWithLines(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	if err := app.RunWithArgs([]string{"log", "--tail", "2", "--lines", "1,2"}); err == nil {
		t.Fatal("expected --tail and --lines error")
	}
}

func TestLogCommandShowsJsonLinesRange(t *testing.T) {
	withTempConfig(t)
	host := core.Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
	for _, command := range []string{"one", "two", "three"} {
		if err := core.AppendRunLog(host, core.RunLogRecord{Target: "devhost", Command: command, Status: "success"}); err != nil {
			t.Fatalf("append log: %v", err)
		}
	}

	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	app := newTestApp()
	if err := app.RunWithArgs([]string{"log", "devhost", "--lines", "2,2"}); err != nil {
		t.Fatalf("log --lines: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `"command":"two"`) || strings.Contains(got, `"command":"one"`) || strings.Contains(got, `"command":"three"`) {
		t.Fatalf("output = %q", got)
	}
}
