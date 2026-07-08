package core

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckHostSuccess(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(keyPath, []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(knownHosts, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(setCheckTCPDialForTest(func(network, address string, timeout time.Duration) (net.Conn, error) {
		return fakeNetConn{}, nil
	}))
	t.Cleanup(setCheckSSHConnectForTest(func(host Host) error {
		return nil
	}))

	result := CheckHost(Host{
		Name:           "devhost",
		IP:             "10.0.0.8",
		User:           "root",
		KeyPath:        keyPath,
		Port:           22,
		KnownHostsPath: knownHosts,
	}, CheckOptions{})

	if result.TCP != CheckStatusOK || result.SSH != CheckStatusOK || result.Auth != CheckStatusOK || result.HostKey != CheckStatusOK || result.Error != "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestCheckHostReportsMissingKey(t *testing.T) {
	result := CheckHost(Host{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: filepath.Join(t.TempDir(), "missing"), Port: 22}, CheckOptions{})
	if result.Error == "" || !strings.Contains(result.Error, "key_path") || result.TCP != CheckStatusSkipped {
		t.Fatalf("result = %+v", result)
	}
}

func TestCheckHostReportsTCPFailure(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(keyPath, []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(setCheckTCPDialForTest(func(network, address string, timeout time.Duration) (net.Conn, error) {
		return nil, errors.New("dial timeout")
	}))

	result := CheckHost(Host{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: keyPath, Port: 22, HostKeyCheck: HostKeyCheckInsecure}, CheckOptions{Timeout: time.Millisecond})
	if result.TCP != CheckStatusFail || !strings.Contains(result.Error, "dial timeout") {
		t.Fatalf("result = %+v", result)
	}
}

func TestCheckHostReportsKnownHostsMissing(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(keyPath, []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(setCheckTCPDialForTest(func(network, address string, timeout time.Duration) (net.Conn, error) {
		return fakeNetConn{}, nil
	}))

	result := CheckHost(Host{Name: "devhost", IP: "10.0.0.8", User: "root", KeyPath: keyPath, Port: 22, KnownHostsPath: filepath.Join(t.TempDir(), "missing_known_hosts")}, CheckOptions{})
	if result.HostKey != CheckStatusFail || !strings.Contains(result.Error, "known_hosts") {
		t.Fatalf("result = %+v", result)
	}
}

func TestCheckHostCommandProxy(t *testing.T) {
	result := CheckHost(Host{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host", RunTemplate: "pct exec 101 -- sh -lc {{cmd}}"}, CheckOptions{})
	if !result.CommandProxy || result.SSH != CheckStatusOK || result.TCP != CheckStatusSkipped || result.Via != "pve-host" || result.Error != "" {
		t.Fatalf("result = %+v", result)
	}
}

func setCheckTCPDialForTest(fn func(string, string, time.Duration) (net.Conn, error)) func() {
	old := checkTCPDial
	checkTCPDial = fn
	return func() { checkTCPDial = old }
}

func setCheckSSHConnectForTest(fn func(Host) error) func() {
	old := checkSSHConnect
	checkSSHConnect = fn
	return func() { checkSSHConnect = old }
}

type fakeNetConn struct{}

func (fakeNetConn) Read([]byte) (int, error)         { return 0, nil }
func (fakeNetConn) Write([]byte) (int, error)        { return 0, nil }
func (fakeNetConn) Close() error                     { return nil }
func (fakeNetConn) LocalAddr() net.Addr              { return fakeAddr("local") }
func (fakeNetConn) RemoteAddr() net.Addr             { return fakeAddr("remote") }
func (fakeNetConn) SetDeadline(time.Time) error      { return nil }
func (fakeNetConn) SetReadDeadline(time.Time) error  { return nil }
func (fakeNetConn) SetWriteDeadline(time.Time) error { return nil }

type fakeAddr string

func (a fakeAddr) Network() string { return string(a) }
func (a fakeAddr) String() string  { return string(a) }
