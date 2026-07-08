package core

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewBatchID(t *testing.T) {
	fixed := time.Date(2026, 7, 8, 12, 1, 2, 123000000, time.Local)
	id := NewBatchID(fixed)
	if !strings.HasPrefix(id, "20260708-120102-") || len(id) != len("20260708-120102-a1b2") {
		t.Fatalf("batch id = %q", id)
	}
}

func TestAppendBatchRunLogWritesJSONL(t *testing.T) {
	withTempConfig(t)
	started := time.Date(2026, 7, 8, 12, 1, 2, 123000000, time.Local)
	record := BatchRunRecord{
		BatchID:   "20260708-120102-a1b2",
		StartedAt: formatLogTime(started),
		EndedAt:   formatLogTime(started.Add(time.Second)),
		Source:    BatchRunSourceLog{Kind: "group", Value: "testing"},
		Command:   "uptime",
		Hosts:     []string{"devhost"},
		Results: []BatchRunResult{{
			Host:       "devhost",
			Status:     "success",
			TaskID:     "task-1",
			DurationMS: 120,
		}},
		SuccessCount: 1,
	}
	if err := AppendBatchRunLog(record); err != nil {
		t.Fatal(err)
	}

	path, err := BatchLogPath(started)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("lines = %#v", lines)
	}
	var got BatchRunRecord
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatal(err)
	}
	if got.BatchID != record.BatchID || got.Source.Kind != "group" || got.Results[0].TaskID != "task-1" {
		t.Fatalf("record = %+v", got)
	}
}

func TestReadBatchRunByID(t *testing.T) {
	withTempConfig(t)
	for _, id := range []string{"old", "new"} {
		if err := AppendBatchRunLog(BatchRunRecord{
			BatchID:   id,
			StartedAt: formatLogTime(time.Date(2026, 7, 8, 12, 1, 2, 0, time.Local)),
			EndedAt:   formatLogTime(time.Date(2026, 7, 8, 12, 1, 3, 0, time.Local)),
			Source:    BatchRunSourceLog{Kind: "hosts", Value: "devhost"},
			Hosts:     []string{"devhost"},
		}); err != nil {
			t.Fatal(err)
		}
	}

	record, ok, err := ReadBatchRunByID("new")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || record.BatchID != "new" {
		t.Fatalf("record=%+v ok=%v", record, ok)
	}
}

func TestMaskRunEnvMasksSensitiveNames(t *testing.T) {
	got := MaskRunEnv(map[string]string{
		"APP_ENV":    "prod",
		"API_TOKEN":  "secret",
		"PRIVATEKEY": "key",
	})
	if got["APP_ENV"] != "prod" || got["API_TOKEN"] != MaskedSecret || got["PRIVATEKEY"] != MaskedSecret {
		t.Fatalf("env = %#v", got)
	}
}
