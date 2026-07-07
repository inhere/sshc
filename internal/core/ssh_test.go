package core

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/melbahja/goph"
	"golang.org/x/crypto/ssh"
)

func TestRemoteClientCloseCallsCloseAll(t *testing.T) {
	called := 0
	client := &remoteClient{
		closeAll: func() error {
			called++
			return nil
		},
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("closeAll calls = %d, want 1", called)
	}
}

func TestRemoteClientCloseAllowsNilClient(t *testing.T) {
	client := &remoteClient{}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewSSHClientWithoutJumpUsesDirectConnection(t *testing.T) {
	restore := setSSHClientFactoriesForTest(
		func(config *goph.Config) (*goph.Client, error) {
			if config.Addr != "10.0.0.8" {
				t.Fatalf("addr = %q, want target", config.Addr)
			}
			return &goph.Client{Config: config}, nil
		},
		newSSHClientConn,
	)
	defer restore()

	client, err := newSSHClientForConnection(ResolvedConnection{Target: sshTestHost("target", "10.0.0.8")}, sshClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("client is nil")
	}
}

func TestNewSSHClientWithJumpWrapsJumpConnectError(t *testing.T) {
	restore := setSSHClientFactoriesForTest(
		func(config *goph.Config) (*goph.Client, error) {
			if config.Addr == "1.2.3.4" {
				return nil, errors.New("jump down")
			}
			return &goph.Client{Config: config}, nil
		},
		newSSHClientConn,
	)
	defer restore()

	_, err := newSSHClientForConnection(sshTestConnection(), sshClientOptions{})
	if err == nil || !strings.Contains(err.Error(), "connect jump host bastion") || !strings.Contains(err.Error(), "jump down") {
		t.Fatalf("err = %v", err)
	}
}

func TestNewSSHClientWithJumpDialsTargetThroughJump(t *testing.T) {
	var dialed string
	restore := setSSHClientFactoriesForTest(
		func(config *goph.Config) (*goph.Client, error) {
			return &goph.Client{Config: config}, nil
		},
		func(conn net.Conn, addr string, config *ssh.ClientConfig) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
			dialed = addr
			return nil, nil, nil, errors.New("handshake failed")
		},
	)
	defer restore()
	oldDial := remoteClientDialForTest
	remoteClientDialForTest = func(client *remoteClient, network, addr string) (net.Conn, error) {
		dialed = addr
		return closeCountingConn{}, nil
	}
	defer func() { remoteClientDialForTest = oldDial }()

	_, err := newSSHClientForConnection(sshTestConnection(), sshClientOptions{})
	if err == nil || !strings.Contains(err.Error(), "connect target host inner-db via jump bastion") {
		t.Fatalf("err = %v", err)
	}
	if dialed != "10.0.0.8:22" {
		t.Fatalf("dialed = %q, want 10.0.0.8:22", dialed)
	}
}

func TestNewSSHClientWithJumpClosesJumpOnTargetError(t *testing.T) {
	closed := 0
	restore := setSSHClientFactoriesForTest(
		func(config *goph.Config) (*goph.Client, error) {
			return &goph.Client{Config: config}, nil
		},
		func(conn net.Conn, addr string, config *ssh.ClientConfig) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
			return nil, nil, nil, errors.New("handshake failed")
		},
	)
	defer restore()

	oldDial := remoteClientDialForTest
	remoteClientDialForTest = func(client *remoteClient, network, addr string) (net.Conn, error) {
		return closeCountingConn{onClose: func() { closed++ }}, nil
	}
	defer func() { remoteClientDialForTest = oldDial }()

	_, _ = newSSHClientForConnection(sshTestConnection(), sshClientOptions{})
	if closed != 1 {
		t.Fatalf("closed raw connections = %d, want 1", closed)
	}
}

func setSSHClientFactoriesForTest(
	newConn func(*goph.Config) (*goph.Client, error),
	newClientConn func(net.Conn, string, *ssh.ClientConfig) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error),
) func() {
	oldNewGophConn := newGophConn
	oldNewSSHClientConn := newSSHClientConn
	newGophConn = newConn
	newSSHClientConn = newClientConn
	return func() {
		newGophConn = oldNewGophConn
		newSSHClientConn = oldNewSSHClientConn
	}
}

func sshTestConnection() ResolvedConnection {
	jump := sshTestHost("bastion", "1.2.3.4")
	return ResolvedConnection{
		Target: sshTestHost("inner-db", "10.0.0.8"),
		Jump:   &jump,
	}
}

func sshTestHost(name, ip string) Host {
	return Host{
		Name:         name,
		IP:           ip,
		User:         "root",
		Password:     "secret",
		Port:         22,
		HostKeyCheck: HostKeyCheckInsecure,
	}
}

type closeCountingConn struct {
	onClose func()
}

func (conn closeCountingConn) Read(_ []byte) (int, error) {
	return 0, errors.New("read not implemented")
}
func (conn closeCountingConn) Write(_ []byte) (int, error) {
	return 0, errors.New("write not implemented")
}
func (conn closeCountingConn) LocalAddr() net.Addr                { return testAddr("local") }
func (conn closeCountingConn) RemoteAddr() net.Addr               { return testAddr("remote") }
func (conn closeCountingConn) SetDeadline(_ time.Time) error      { return nil }
func (conn closeCountingConn) SetReadDeadline(_ time.Time) error  { return nil }
func (conn closeCountingConn) SetWriteDeadline(_ time.Time) error { return nil }
func (conn closeCountingConn) Close() error {
	if conn.onClose != nil {
		conn.onClose()
	}
	return nil
}

type testAddr string

func (addr testAddr) Network() string { return string(addr) }
func (addr testAddr) String() string  { return string(addr) }
