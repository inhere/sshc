package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/inhere/sshc/internal/core"
)

func TestTerminalSessionLifecycle(t *testing.T) {
	withTempConfig(t)
	t.Cleanup(core.SetNowForTest(func() time.Time {
		return time.Date(2026, 7, 7, 12, 13, 14, 123000000, time.Local)
	}))
	if err := core.SaveConfig(&core.Config{
		Hosts: []core.Host{{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22, HostKeyCheck: core.HostKeyCheckInsecure}},
	}); err != nil {
		t.Fatal(err)
	}
	fake := newFakeWebPTY()
	t.Cleanup(setOpenTerminalPTYForTest(func(host core.Host, opts core.WebPTYOptions) (core.WebPTY, error) {
		if host.Name != "devhost" || opts.Cols != 100 || opts.Rows != 30 {
			t.Fatalf("open host=%+v opts=%+v", host, opts)
		}
		return fake, nil
	}))
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0"})

	rec := requestJSON(t, srv, http.MethodPost, "/api/terminal/sessions", `{"host":"devhost","cols":100,"rows":30}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create response = %d %s", rec.Code, rec.Body.String())
	}
	var created terminalSessionInfo
	decodeResponseData(t, rec, &created)
	if !strings.HasPrefix(created.ID, "term_") || created.Host != "devhost" {
		t.Fatalf("created = %+v", created)
	}

	rec = requestJSON(t, srv, http.MethodGet, "/api/terminal/sessions", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), created.ID) {
		t.Fatalf("list response = %d %s", rec.Code, rec.Body.String())
	}
	rec = requestJSON(t, srv, http.MethodPost, "/api/terminal/sessions/"+created.ID+"/resize", `{"cols":120,"rows":40}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("resize response = %d %s", rec.Code, rec.Body.String())
	}
	if fake.cols != 120 || fake.rows != 40 {
		t.Fatalf("resize = %dx%d", fake.cols, fake.rows)
	}
	rec = requestJSON(t, srv, http.MethodDelete, "/api/terminal/sessions/"+created.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete response = %d %s", rec.Code, rec.Body.String())
	}
	select {
	case <-fake.closed:
	case <-time.After(time.Second):
		t.Fatal("fake pty not closed")
	}
	auditPath, err := terminalAuditPath(core.Now())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(auditPath) != "20260707.jsonl" {
		t.Fatalf("audit path = %s", auditPath)
	}
}

func TestTerminalCreateReadonly(t *testing.T) {
	withTempConfig(t)
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0", Readonly: true})
	rec := requestJSON(t, srv, http.MethodPost, "/api/terminal/sessions", `{"host":"devhost"}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "readonly") {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestTerminalWebSocketTransfersAndCleansUp(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		Hosts: []core.Host{{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22, HostKeyCheck: core.HostKeyCheckInsecure}},
	}); err != nil {
		t.Fatal(err)
	}
	fake := newFakeWebPTY()
	t.Cleanup(setOpenTerminalPTYForTest(func(core.Host, core.WebPTYOptions) (core.WebPTY, error) {
		return fake, nil
	}))
	srv := newTestServer(t, Config{Addr: "127.0.0.1:0"})
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	rec := requestJSON(t, srv, http.MethodPost, "/api/terminal/sessions", `{"host":"devhost"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create response = %d %s", rec.Code, rec.Body.String())
	}
	var created terminalSessionInfo
	decodeResponseData(t, rec, &created)

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/api/terminal/sessions/" + created.ID + "/ws"
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	go func() {
		_, _ = fake.outputW.Write([]byte("hello"))
	}()
	_, data, err := conn.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("ws output = %q", string(data))
	}
	if err := conn.Write(context.Background(), websocket.MessageText, []byte("pwd\n")); err != nil {
		t.Fatal(err)
	}
	if !fake.waitInput("pwd\n", time.Second) {
		t.Fatalf("pty input = %q", fake.inputString())
	}
	if err := conn.Close(websocket.StatusNormalClosure, "done"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(time.Second)
	for {
		if len(srv.terminals.list()) == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("session not cleaned up: %+v", srv.terminals.list())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

type fakeWebPTY struct {
	outputR *io.PipeReader
	outputW *io.PipeWriter
	closed  chan struct{}

	mu         sync.Mutex
	input      bytes.Buffer
	cols       int
	rows       int
	closedOnce sync.Once
}

func newFakeWebPTY() *fakeWebPTY {
	r, w := io.Pipe()
	return &fakeWebPTY{outputR: r, outputW: w, closed: make(chan struct{})}
}

func (p *fakeWebPTY) Read(buf []byte) (int, error) {
	return p.outputR.Read(buf)
}

func (p *fakeWebPTY) Write(buf []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.input.Write(buf)
}

func (p *fakeWebPTY) Resize(cols, rows int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cols = cols
	p.rows = rows
	return nil
}

func (p *fakeWebPTY) Close() error {
	p.closedOnce.Do(func() {
		_ = p.outputR.Close()
		_ = p.outputW.Close()
		close(p.closed)
	})
	return nil
}

func (p *fakeWebPTY) Wait() error {
	<-p.closed
	return nil
}

func (p *fakeWebPTY) waitInput(want string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(p.inputString(), want) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func (p *fakeWebPTY) inputString() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.input.String()
}

func setOpenTerminalPTYForTest(fn func(core.Host, core.WebPTYOptions) (core.WebPTY, error)) func() {
	old := openTerminalPTY
	openTerminalPTY = fn
	return func() { openTerminalPTY = old }
}

var errFakePTYClosed = errors.New("fake pty closed")
