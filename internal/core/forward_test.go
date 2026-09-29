package core

import (
	"errors"
	"io"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeForwardDialer struct {
	mu        sync.Mutex
	dials     []string
	dialFn    func(network, addr string) (net.Conn, error)
	keepalive func() error
	waitCh    chan error
	closed    chan struct{}
	closeOnce sync.Once
}

func newFakeForwardDialer() *fakeForwardDialer {
	return &fakeForwardDialer{
		waitCh: make(chan error, 1),
		closed: make(chan struct{}),
	}
}

func (f *fakeForwardDialer) Dial(network, addr string) (net.Conn, error) {
	f.mu.Lock()
	f.dials = append(f.dials, addr)
	f.mu.Unlock()
	if f.dialFn == nil {
		return nil, errors.New("fake dialer: dial not configured")
	}
	return f.dialFn(network, addr)
}

func (f *fakeForwardDialer) SendKeepalive() error {
	if f.keepalive == nil {
		return nil
	}
	return f.keepalive()
}

func (f *fakeForwardDialer) Wait() error {
	select {
	case err := <-f.waitCh:
		return err
	case <-f.closed:
		return nil
	}
}

func (f *fakeForwardDialer) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

func (f *fakeForwardDialer) dialed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.dials...)
}

func (f *fakeForwardDialer) isClosed() bool {
	select {
	case <-f.closed:
		return true
	default:
		return false
	}
}

// setForwardDialerForTest installs a fake session for the next StartLocalForward.
func setForwardDialerForTest(dialer forwardDialer) func() {
	old := newForwardDialerForTest
	newForwardDialerForTest = func(Host) (forwardDialer, error) { return dialer, nil }
	return func() { newForwardDialerForTest = old }
}

// startEchoServer starts a loopback TCP echo server and returns its address.
func startEchoServer(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen echo server: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return listener.Addr().String()
}

func forwardTestHost() Host {
	return Host{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}
}

func TestStartLocalForwardForwardsAndHalfCloses(t *testing.T) {
	baseline := runtime.NumGoroutine()
	echoAddr := startEchoServer(t)
	dialer := newFakeForwardDialer()
	dialer.dialFn = func(_, _ string) (net.Conn, error) { return net.Dial("tcp", echoAddr) }
	defer setForwardDialerForTest(dialer)()

	session, err := StartLocalForward(forwardTestHost(), []ForwardRule{{
		LocalAddr:  "127.0.0.1:0",
		RemoteAddr: "127.0.0.1:6379",
	}}, ForwardOptions{})
	if err != nil {
		t.Fatalf("StartLocalForward: %v", err)
	}
	endpoints := session.Endpoints()
	if len(endpoints) != 1 || strings.HasSuffix(endpoints[0], ":0") {
		t.Fatalf("endpoints = %v, want one resolved local address", endpoints)
	}

	client, err := net.Dial("tcp", endpoints[0])
	if err != nil {
		t.Fatalf("dial local endpoint: %v", err)
	}
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("read echoed bytes: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("echoed %q, want ping", string(buf))
	}

	tcpClient, ok := client.(*net.TCPConn)
	if !ok {
		t.Fatalf("client conn type = %T", client)
	}
	if err := tcpClient.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite: %v", err)
	}
	_ = tcpClient.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := client.Read(buf); !errors.Is(err, io.EOF) {
		t.Fatalf("read after half close = %v, want EOF", err)
	}
	_ = client.Close()

	if got := dialer.dialed(); len(got) != 1 || got[0] != "127.0.0.1:6379" {
		t.Fatalf("dialed = %v, want the configured remote endpoint", got)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := session.Wait(); err != nil {
		t.Fatalf("Wait after Close = %v, want nil", err)
	}
	if !dialer.isClosed() {
		t.Fatal("session did not close the ssh dialer")
	}
	assertGoroutinesSettle(t, baseline)
}

func TestForwardSessionDetectsSessionLoss(t *testing.T) {
	baseline := runtime.NumGoroutine()
	dialer := newFakeForwardDialer()
	defer setForwardDialerForTest(dialer)()

	session, err := StartLocalForward(forwardTestHost(), []ForwardRule{{
		LocalAddr:  "127.0.0.1:0",
		RemoteAddr: "127.0.0.1:6379",
	}}, ForwardOptions{KeepaliveEvery: time.Hour})
	if err != nil {
		t.Fatalf("StartLocalForward: %v", err)
	}
	endpoints := session.Endpoints()
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %v", endpoints)
	}

	dialer.waitCh <- errors.New("connection reset by peer")

	waitErr := make(chan error, 1)
	go func() { waitErr <- session.Wait() }()
	select {
	case err := <-waitErr:
		if err == nil || !strings.Contains(err.Error(), "connection reset by peer") {
			t.Fatalf("Wait = %v, want the session loss error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("session did not stop after the ssh session was lost")
	}
	if !dialer.isClosed() {
		t.Fatal("session loss did not close the ssh dialer")
	}
	if _, err := net.DialTimeout("tcp", endpoints[0], 200*time.Millisecond); err == nil {
		t.Fatal("listener still accepts connections after session loss")
	}
	assertGoroutinesSettle(t, baseline)
}

func TestForwardSessionDetectsKeepaliveFailure(t *testing.T) {
	baseline := runtime.NumGoroutine()
	dialer := newFakeForwardDialer()
	dialer.keepalive = func() error { return errors.New("keepalive rejected") }
	defer setForwardDialerForTest(dialer)()

	session, err := StartLocalForward(forwardTestHost(), []ForwardRule{{
		LocalAddr:  "127.0.0.1:0",
		RemoteAddr: "127.0.0.1:6379",
	}}, ForwardOptions{KeepaliveEvery: 10 * time.Millisecond, KeepaliveWait: time.Second})
	if err != nil {
		t.Fatalf("StartLocalForward: %v", err)
	}

	waitErr := make(chan error, 1)
	go func() { waitErr <- session.Wait() }()
	select {
	case err := <-waitErr:
		if err == nil || !strings.Contains(err.Error(), "keepalive") {
			t.Fatalf("Wait = %v, want a keepalive error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("session did not stop after keepalive failure")
	}
	assertGoroutinesSettle(t, baseline)
}

func TestStartLocalForwardRollsBackOnListenerFailure(t *testing.T) {
	baseline := runtime.NumGoroutine()
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen busy port: %v", err)
	}
	defer busy.Close()

	dialer := newFakeForwardDialer()
	defer setForwardDialerForTest(dialer)()

	session, err := StartLocalForward(forwardTestHost(), []ForwardRule{
		{LocalAddr: "127.0.0.1:0", RemoteAddr: "127.0.0.1:6379"},
		{LocalAddr: busy.Addr().String(), RemoteAddr: "127.0.0.1:6379"},
	}, ForwardOptions{})
	if err == nil {
		_ = session.Close()
		t.Fatal("expected a listen error for the busy local port")
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Fatalf("err = %v, want a listen error", err)
	}
	if !dialer.isClosed() {
		t.Fatal("rollback did not close the ssh dialer")
	}
	assertGoroutinesSettle(t, baseline)
}

func TestStartLocalForwardRejectsInvalidRules(t *testing.T) {
	dialer := newFakeForwardDialer()
	defer setForwardDialerForTest(dialer)()

	cases := []struct {
		name  string
		rules []ForwardRule
		want  string
	}{
		{"no rules", nil, "at least one forward rule"},
		{"wildcard local", []ForwardRule{{LocalAddr: "0.0.0.0:15432", RemoteAddr: "127.0.0.1:5432"}}, "loopback"},
		{"lan local", []ForwardRule{{LocalAddr: "192.168.1.5:15432", RemoteAddr: "127.0.0.1:5432"}}, "loopback"},
		{"remote port zero", []ForwardRule{{LocalAddr: "127.0.0.1:15432", RemoteAddr: "127.0.0.1:0"}}, "port"},
		{"duplicate local", []ForwardRule{
			{LocalAddr: "127.0.0.1:15432", RemoteAddr: "127.0.0.1:5432"},
			{LocalAddr: "127.0.0.1:15432", RemoteAddr: "127.0.0.1:6379"},
		}, "duplicate local"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			session, err := StartLocalForward(forwardTestHost(), tc.rules, ForwardOptions{})
			if err == nil {
				_ = session.Close()
				t.Fatalf("expected an error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestForwardSessionCloseReleasesLocalPort(t *testing.T) {
	dialer := newFakeForwardDialer()
	defer setForwardDialerForTest(dialer)()

	session, err := StartLocalForward(forwardTestHost(), []ForwardRule{{
		LocalAddr:  "127.0.0.1:0",
		RemoteAddr: "127.0.0.1:5432",
	}}, ForwardOptions{})
	if err != nil {
		t.Fatalf("StartLocalForward: %v", err)
	}
	endpoints := session.Endpoints()
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %v", endpoints)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Close must be idempotent and the captured port must be reusable immediately.
	if err := session.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if err := session.Wait(); err != nil {
		t.Fatalf("Wait = %v, want nil", err)
	}
	rebound, err := net.Listen("tcp", endpoints[0])
	if err != nil {
		t.Fatalf("local port %s was not released: %v", endpoints[0], err)
	}
	_ = rebound.Close()
}

func assertGoroutinesSettle(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		current := runtime.NumGoroutine()
		if current <= baseline+2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutines did not settle: baseline=%d current=%d", baseline, current)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
