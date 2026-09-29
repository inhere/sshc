package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/inhere/sshc/internal/core"
)

type fakeForwardSession struct {
	endpoints  []string
	err        error
	closeCalls int32
	closed     chan struct{}
	closeOnce  func()
}

func newFakeForwardSession(endpoints ...string) *fakeForwardSession {
	session := &fakeForwardSession{endpoints: endpoints, closed: make(chan struct{})}
	var once int32
	session.closeOnce = func() {
		if atomic.CompareAndSwapInt32(&once, 0, 1) {
			atomic.AddInt32(&session.closeCalls, 1)
			close(session.closed)
		}
	}
	return session
}

func (f *fakeForwardSession) Endpoints() []string { return f.endpoints }

func (f *fakeForwardSession) Wait() error {
	if f.err != nil {
		return f.err
	}
	<-f.closed
	return nil
}

func (f *fakeForwardSession) Close() error {
	f.closeOnce()
	return nil
}

func tunnelCommandConfig(t *testing.T) {
	t.Helper()
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}},
		Hosts: []core.Host{
			{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22},
			{Name: "bastion", IP: "10.0.0.9", User: "root", KeyPath: "~/.ssh/id_rsa", Port: 22},
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func readTunnels(t *testing.T) []core.TunnelProfile {
	t.Helper()
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return config.Tunnels
}

func TestTunnelAddAndList(t *testing.T) {
	withTempConfig(t)
	tunnelCommandConfig(t)
	app := newTestApp()

	if err := app.RunWithArgs([]string{
		"tunnel", "add", "dev-db",
		"--target", "devho",
		"--auth", "dev-root",
		"--jump", "bastion",
		"--forward", "15432=127.0.0.1:5432",
		"--forward", "16379=6379",
	}); err != nil {
		t.Fatalf("tunnel add: %v", err)
	}

	tunnels := readTunnels(t)
	if len(tunnels) != 1 {
		t.Fatalf("tunnels = %+v", tunnels)
	}
	profile := tunnels[0]
	if profile.Name != "dev-db" || profile.Target != "devhost" || profile.AuthRef != "dev-root" || profile.Jump != "bastion" {
		t.Fatalf("profile = %+v", profile)
	}
	if len(profile.Forwards) != 2 {
		t.Fatalf("forwards = %+v", profile.Forwards)
	}
	if profile.Forwards[0].Local != "127.0.0.1:15432" || profile.Forwards[0].Remote != "127.0.0.1:5432" {
		t.Fatalf("forward[0] = %+v", profile.Forwards[0])
	}
	if profile.Forwards[1].Remote != "127.0.0.1:6379" {
		t.Fatalf("forward[1] = %+v", profile.Forwards[1])
	}

	// duplicate without --force is refused, with --force it updates.
	if err := app.RunWithArgs([]string{"tunnel", "add", "dev-db", "--target", "devhost", "--forward", "15433=5432"}); err == nil {
		t.Fatal("expected a duplicate tunnel error")
	} else if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("err = %v, want a --force hint", err)
	}
	if err := app.RunWithArgs([]string{"tunnel", "add", "dev-db", "--target", "devhost", "--forward", "15433=5432", "--force"}); err != nil {
		t.Fatalf("tunnel add --force: %v", err)
	}
	if tunnels = readTunnels(t); len(tunnels) != 1 || tunnels[0].Forwards[0].Local != "127.0.0.1:15433" {
		t.Fatalf("tunnels after force = %+v", tunnels)
	}

	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	if err := app.RunWithArgs([]string{"tunnel", "list"}); err != nil {
		t.Fatalf("tunnel list: %v", err)
	}
	if !strings.Contains(out.String(), "dev-db") || !strings.Contains(out.String(), "host") {
		t.Fatalf("list output = %q", out.String())
	}

	out.Reset()
	if err := app.RunWithArgs([]string{"tunnel", "list", "--json"}); err != nil {
		t.Fatalf("tunnel list --json: %v", err)
	}
	var listed []core.TunnelProfile
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil {
		t.Fatalf("list json %q: %v", out.String(), err)
	}
	if len(listed) != 1 || listed[0].Name != "dev-db" {
		t.Fatalf("listed = %+v", listed)
	}

	out.Reset()
	if err := app.RunWithArgs([]string{"tunnel", "show", "dev-db"}); err != nil {
		t.Fatalf("tunnel show: %v", err)
	}
	if !strings.Contains(out.String(), "target: devhost") || !strings.Contains(out.String(), "forward: 127.0.0.1:15433 -> 127.0.0.1:5432") {
		t.Fatalf("show output = %q", out.String())
	}

	if err := app.RunWithArgs([]string{"tunnel", "rm", "dev-db", "--yes"}); err != nil {
		t.Fatalf("tunnel rm: %v", err)
	}
	if tunnels = readTunnels(t); len(tunnels) != 0 {
		t.Fatalf("tunnels after rm = %+v", tunnels)
	}
	if err := app.RunWithArgs([]string{"tunnel", "show", "dev-db"}); err == nil {
		t.Fatal("expected an error for an unknown tunnel")
	}
}

func TestTunnelAddRejectsInvalidInput(t *testing.T) {
	withTempConfig(t)
	tunnelCommandConfig(t)
	app := newTestApp()

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no target", []string{"tunnel", "add", "t", "--forward", "15432=5432"}, "--target or --address"},
		{"both targets", []string{"tunnel", "add", "t", "--target", "devhost", "--address", "192.168.1.20", "--auth", "dev-root", "--forward", "15432=5432"}, "mutually exclusive"},
		{"address without auth", []string{"tunnel", "add", "t", "--address", "192.168.1.20", "--forward", "15432=5432"}, "--auth is required"},
		{"unknown host", []string{"tunnel", "add", "t", "--target", "missing", "--forward", "15432=5432"}, "not found"},
		{"unknown auth", []string{"tunnel", "add", "t", "--target", "devhost", "--auth", "missing", "--forward", "15432=5432"}, "auth profile"},
		{"no forwards", []string{"tunnel", "add", "t", "--target", "devhost"}, "at least one --forward"},
		{"non loopback local", []string{"tunnel", "add", "t", "--target", "devhost", "--forward", "192.168.1.5:15432=5432"}, "loopback"},
		{"bad rule", []string{"tunnel", "add", "t", "--target", "devhost", "--forward", "15432:127.0.0.1:5432"}, "want local=remote"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := app.RunWithArgs(tc.args)
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
	if tunnels := readTunnels(t); len(tunnels) != 0 {
		t.Fatalf("invalid input must not be saved: %+v", tunnels)
	}
}

func TestTunnelForwardJSONReadyOutput(t *testing.T) {
	withTempConfig(t)
	tunnelCommandConfig(t)
	app := newTestApp()

	session := newFakeForwardSession("127.0.0.1:54321")
	var gotHost core.Host
	var gotRules []core.ForwardRule
	t.Cleanup(setStartLocalForwardForTest(func(host core.Host, rules []core.ForwardRule, _ core.ForwardOptions) (core.ForwardSession, error) {
		gotHost = host
		gotRules = rules
		return session, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	t.Cleanup(setNotifyContextForTest(func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
		return ctx, func() {}
	}))

	var out bytes.Buffer
	var status bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	t.Cleanup(setStatusOutputForTest(&status))

	if err := app.RunWithArgs([]string{
		"tunnel", "forward",
		"--target", "devhost", "--auth", "dev-root",
		"--forward", "0=127.0.0.1:5432",
		"--json",
	}); err != nil {
		t.Fatalf("tunnel forward --json: %v", err)
	}
	if status.String() != "" {
		t.Fatalf("status = %q, want it empty in --json mode", status.String())
	}

	if gotHost.Name != "devhost" || gotHost.IP != "10.0.0.8" {
		t.Fatalf("forwarded host = %+v", gotHost)
	}
	if len(gotRules) != 1 || gotRules[0].LocalAddr != "127.0.0.1:0" || gotRules[0].RemoteAddr != "127.0.0.1:5432" {
		t.Fatalf("rules = %+v", gotRules)
	}
	if calls := atomic.LoadInt32(&session.closeCalls); calls != 1 {
		t.Fatalf("session close calls = %d, want 1 on interrupt", calls)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("stdout = %q, want exactly one json line", out.String())
	}
	var payload struct {
		Name      string `json:"name"`
		Target    string `json:"target"`
		Listeners []struct {
			Local  string `json:"local"`
			Remote string `json:"remote"`
		} `json:"listeners"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &payload); err != nil {
		t.Fatalf("ready json %q: %v", lines[0], err)
	}
	if payload.Target != "devhost" || len(payload.Listeners) != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Listeners[0].Local != "127.0.0.1:54321" || payload.Listeners[0].Remote != "127.0.0.1:5432" {
		t.Fatalf("listener = %+v", payload.Listeners[0])
	}
}

func TestTunnelForwardWithoutJSONKeepsStdoutClean(t *testing.T) {
	withTempConfig(t)
	tunnelCommandConfig(t)
	app := newTestApp()

	session := newFakeForwardSession("127.0.0.1:54322")
	t.Cleanup(setStartLocalForwardForTest(func(core.Host, []core.ForwardRule, core.ForwardOptions) (core.ForwardSession, error) {
		return session, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	t.Cleanup(setNotifyContextForTest(func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
		return ctx, func() {}
	}))

	var out bytes.Buffer
	var status bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	t.Cleanup(setStatusOutputForTest(&status))
	if err := app.RunWithArgs([]string{"tunnel", "forward", "dev-db"}); err == nil {
		t.Fatal("expected tunnel not found for an unsaved name")
	}

	if err := app.RunWithArgs([]string{"tunnel", "add", "dev-db", "--target", "devhost", "--forward", "15432=5432"}); err != nil {
		t.Fatalf("tunnel add: %v", err)
	}
	out.Reset()
	if err := app.RunWithArgs([]string{"tunnel", "forward", "dev-db"}); err != nil {
		t.Fatalf("tunnel forward: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q, want it empty without --json", out.String())
	}
	if !strings.Contains(status.String(), "tunnel ready 127.0.0.1:54322 -> 127.0.0.1:5432") {
		t.Fatalf("status = %q, want the ready line on the status stream", status.String())
	}
	if calls := atomic.LoadInt32(&session.closeCalls); calls != 1 {
		t.Fatalf("session close calls = %d, want 1", calls)
	}
}

func TestTunnelForwardFailsWhenSessionFails(t *testing.T) {
	withTempConfig(t)
	tunnelCommandConfig(t)
	app := newTestApp()

	session := newFakeForwardSession("127.0.0.1:54323")
	session.err = errors.New("ssh session closed")
	t.Cleanup(setStartLocalForwardForTest(func(core.Host, []core.ForwardRule, core.ForwardOptions) (core.ForwardSession, error) {
		return session, nil
	}))
	t.Cleanup(setNotifyContextForTest(func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
		return context.WithCancel(parent)
	}))

	err := app.RunWithArgs([]string{"tunnel", "forward", "--address", "192.168.1.20", "--auth", "dev-root", "--forward", "16379=6379"})
	if err == nil || !strings.Contains(err.Error(), "ssh session closed") {
		t.Fatalf("err = %v, want the session failure", err)
	}
}

func TestTunnelForwardRejectsCommandProxyTarget(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		Hosts: []core.Host{{Name: "lxc-app", Backend: core.HostBackendCommandProxy, Via: "devhost", RunTemplate: "pct exec 100 -- {{cmd}}"}},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	app := newTestApp()
	err := app.RunWithArgs([]string{"tunnel", "forward", "--target", "lxc-app", "--forward", "15432=5432"})
	if err == nil || !strings.Contains(err.Error(), "command_proxy") {
		t.Fatalf("err = %v, want a command_proxy error", err)
	}
}
