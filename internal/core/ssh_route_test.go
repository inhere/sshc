package core

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestNewSSHClientWithDialAddressUsesTargetIdentity(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{})
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			close(accepted)
			_ = conn.Close()
		}
	}()

	var handshakeAddress string
	restore := setSSHClientFactoriesForTest(
		func(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
			return nil, errors.New("unexpected direct dial")
		},
		func(conn net.Conn, addr string, config *ssh.ClientConfig) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
			handshakeAddress = addr
			return nil, nil, nil, errors.New("handshake failed")
		},
	)
	defer restore()

	host := sshTestHost("target", "10.0.0.8")
	_, err = newSSHClientForConnection(ResolvedConnection{Target: host}, sshClientOptions{DialAddress: listener.Addr().String()})
	if err == nil || !strings.Contains(err.Error(), "handshake failed") {
		t.Fatalf("err = %v", err)
	}
	if handshakeAddress != "10.0.0.8:22" {
		t.Fatalf("handshake address = %q", handshakeAddress)
	}
	if !receiveWithin(accepted, time.Second) {
		t.Fatal("local dial address was not used")
	}
}
