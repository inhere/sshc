package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/inhere/sshc/internal/core"
)

type terminalManager struct {
	mu       sync.Mutex
	sessions map[string]*terminalSession
}

type terminalSession struct {
	ID         string    `json:"id"`
	Host       string    `json:"host"`
	RemoteAddr string    `json:"remote_addr,omitempty"`
	StartedAt  time.Time `json:"started_at"`

	pty       core.WebPTY
	closeOnce sync.Once
	closed    chan struct{}
	closeErr  error
}

type terminalSessionInfo struct {
	ID         string `json:"id"`
	Host       string `json:"host"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	StartedAt  string `json:"started_at"`
}

type terminalCreateRequest struct {
	Host string `json:"host"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
	Term string `json:"term"`
}

type terminalResizeRequest struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

var (
	errTerminalSessionNotFound = errors.New("terminal session not found")
	openTerminalPTY            = core.OpenWebPTY
)

func newTerminalManager() *terminalManager {
	return &terminalManager{sessions: make(map[string]*terminalSession)}
}

func (m *terminalManager) create(ctx context.Context, req terminalCreateRequest, remoteAddr string) (*terminalSession, error) {
	target := strings.TrimSpace(req.Host)
	if target == "" {
		return nil, errors.New("host is required")
	}
	host, ok, err := core.ResolveHostWithSSHConfig(target, core.HostOverrides{})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("host %q not found", target)
	}
	pty, err := openTerminalPTY(host, core.WebPTYOptions{Cols: req.Cols, Rows: req.Rows, Term: req.Term})
	if err != nil {
		return nil, terminalConnectError(host, err)
	}
	token, err := GenerateToken()
	if err != nil {
		_ = pty.Close()
		return nil, err
	}
	session := &terminalSession{
		ID:         "term_" + token,
		Host:       core.HostLogName(host),
		RemoteAddr: remoteAddr,
		StartedAt:  core.Now(),
		pty:        pty,
		closed:     make(chan struct{}),
	}
	m.mu.Lock()
	m.sessions[session.ID] = session
	m.mu.Unlock()
	_ = appendTerminalAudit(ctx, terminalAuditRecord{
		SessionID:  session.ID,
		Host:       session.Host,
		RemoteAddr: remoteAddr,
		Event:      "start",
		Message:    "terminal session started",
	})
	return session, nil
}

func (m *terminalManager) list() []terminalSessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	infos := make([]terminalSessionInfo, 0, len(m.sessions))
	for _, session := range m.sessions {
		infos = append(infos, session.info())
	}
	return infos
}

func (m *terminalManager) get(id string) (*terminalSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[strings.TrimSpace(id)]
	return session, ok
}

func (m *terminalManager) resize(ctx context.Context, id string, cols, rows int) error {
	session, ok := m.get(id)
	if !ok {
		return fmt.Errorf("%w: %q", errTerminalSessionNotFound, id)
	}
	if cols < 1 || rows < 1 {
		return errors.New("cols and rows must be greater than 0")
	}
	if err := session.pty.Resize(cols, rows); err != nil {
		return err
	}
	return appendTerminalAudit(ctx, terminalAuditRecord{
		SessionID:  session.ID,
		Host:       session.Host,
		RemoteAddr: session.RemoteAddr,
		Event:      "resize",
		Cols:       cols,
		Rows:       rows,
	})
}

func (m *terminalManager) close(ctx context.Context, id, reason string) error {
	m.mu.Lock()
	session, ok := m.sessions[strings.TrimSpace(id)]
	if ok {
		delete(m.sessions, session.ID)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %q", errTerminalSessionNotFound, id)
	}
	err := session.close()
	_ = appendTerminalAudit(ctx, terminalAuditRecord{
		SessionID:  session.ID,
		Host:       session.Host,
		RemoteAddr: session.RemoteAddr,
		Event:      "close",
		Message:    reason,
	})
	return err
}

func (m *terminalManager) closeIfExists(ctx context.Context, id, reason string) error {
	err := m.close(ctx, id, reason)
	if errors.Is(err, errTerminalSessionNotFound) {
		return nil
	}
	return err
}

func (s *terminalSession) info() terminalSessionInfo {
	return terminalSessionInfo{
		ID:         s.ID,
		Host:       s.Host,
		RemoteAddr: s.RemoteAddr,
		StartedAt:  s.StartedAt.Format("2006-01-02T15:04:05.000"),
	}
}

func (s *terminalSession) close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.pty.Close()
		close(s.closed)
	})
	return s.closeErr
}

func terminalConnectError(host core.Host, err error) error {
	if strings.Contains(err.Error(), "knownhosts: key is unknown") {
		return fmt.Errorf("host key is unknown for %s; run `sshc host trust %s` first", core.HostLogName(host), core.HostLogName(host))
	}
	return err
}

func copyWebSocketToPTY(ctx context.Context, pty io.Writer, read func(context.Context) ([]byte, error)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		data, err := read(ctx)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			continue
		}
		if _, err := pty.Write(data); err != nil {
			return err
		}
	}
}
