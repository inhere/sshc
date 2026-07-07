package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/inhere/sshc/internal/core"
)

const terminalAuditTimeLayout = "2006-01-02T15:04:05.000"

type terminalAuditRecord struct {
	SessionID  string
	Host       string
	RemoteAddr string
	Event      string
	Message    string
	Cols       int
	Rows       int
}

func appendTerminalAudit(ctx context.Context, rec terminalAuditRecord) error {
	logTime := core.Now()
	path, err := terminalAuditPath(logTime)
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
		ReplaceAttr: replaceTerminalAuditTimeAttr,
	})
	record := slog.NewRecord(logTime, slog.LevelInfo, "terminal", 0)
	record.AddAttrs(
		slog.String("session_id", rec.SessionID),
		slog.String("host", rec.Host),
		slog.String("remote_addr", rec.RemoteAddr),
		slog.String("event", rec.Event),
	)
	if rec.Message != "" {
		record.AddAttrs(slog.String("message", rec.Message))
	}
	if rec.Cols > 0 {
		record.AddAttrs(slog.Int("cols", rec.Cols))
	}
	if rec.Rows > 0 {
		record.AddAttrs(slog.Int("rows", rec.Rows))
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return handler.Handle(ctx, record)
}

func terminalAuditPath(logTime time.Time) (string, error) {
	dir, err := core.LogsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "terminal", logTime.Format("20060102")+".jsonl"), nil
}

func replaceTerminalAuditTimeAttr(groups []string, attr slog.Attr) slog.Attr {
	if attr.Key == slog.TimeKey {
		if value, ok := attr.Value.Any().(time.Time); ok {
			attr.Value = slog.StringValue(value.Format(terminalAuditTimeLayout))
		}
	}
	return attr
}
