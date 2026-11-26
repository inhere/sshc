package core

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gookit/goutil/x/assert"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestSessionInputWriterTimesOutBlockedWrite(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	input := sessionInputWriter{
		writer:  writer,
		timeout: 10 * time.Millisecond,
		abort:   reader.Close,
	}

	_, err := input.Write([]byte("x"))
	assert.Err(t, err)
}

func TestSendSSHKeepaliveUsesGlobalRequest(t *testing.T) {
	var gotName string
	var gotWantReply bool
	var gotPayload []byte
	errWant := errors.New("keepalive failed")
	client := fakeKeepaliveClient{
		send: func(name string, wantReply bool, payload []byte) (bool, []byte, error) {
			gotName = name
			gotWantReply = wantReply
			gotPayload = payload
			return false, nil, errWant
		},
	}

	err := sendSSHKeepalive(client)
	if !errors.Is(err, errWant) {
		t.Fatalf("err = %v, want %v", err, errWant)
	}
	if gotName != "keepalive@openssh.com" || !gotWantReply || gotPayload != nil {
		t.Fatalf("request = name %q, wantReply %t, payload %#v", gotName, gotWantReply, gotPayload)
	}
}

type fakeKeepaliveClient struct {
	send func(string, bool, []byte) (bool, []byte, error)
}

func (f fakeKeepaliveClient) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
	return f.send(name, wantReply, payload)
}

func TestNormalizeLoginWaitErrorTreatsMissingExitStatusAsCleanClose(t *testing.T) {
	if err := normalizeLoginWaitError(&ssh.ExitMissingError{}); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestNormalizeLoginWaitErrorPreservesOtherErrors(t *testing.T) {
	errWant := errors.New("connection lost")
	if err := normalizeLoginWaitError(errWant); !errors.Is(err, errWant) {
		t.Fatalf("err = %v, want %v", err, errWant)
	}
}

func TestNormalizeLoginWaitErrorMapsRemoteExitStatus(t *testing.T) {
	t.Run("exit status becomes remote exit error", func(t *testing.T) {
		status := uint32(130)
		client := dialTestSSHClient(t, startSessionExitServer(t, &status))

		err := runTestSSHCommand(t, client, "exit 130")
		var remoteExit *RemoteExitError
		if !errors.As(normalizeLoginWaitError(err), &remoteExit) {
			t.Fatalf("err = %v, want RemoteExitError", err)
		}
		if remoteExit.Status != 130 {
			t.Fatalf("status = %d, want 130", remoteExit.Status)
		}
	})

	t.Run("close without exit status stays silent", func(t *testing.T) {
		client := dialTestSSHClient(t, startSessionExitServer(t, nil))

		err := runTestSSHCommand(t, client, "exit 0")
		var missingExitStatus *ssh.ExitMissingError
		if !errors.As(err, &missingExitStatus) {
			t.Fatalf("err = %v, want ExitMissingError", err)
		}
		if got := normalizeLoginWaitError(err); got != nil {
			t.Fatalf("err = %v, want nil", got)
		}
	})
}

func TestPendingExitCodeRoundTrip(t *testing.T) {
	TakeExitCode()
	defer TakeExitCode()

	if got := TakeExitCode(); got != 0 {
		t.Fatalf("initial exit code = %d, want 0", got)
	}
	SetExitCode(130)
	if got := TakeExitCode(); got != 130 {
		t.Fatalf("exit code = %d, want 130", got)
	}
	if got := TakeExitCode(); got != 0 {
		t.Fatalf("exit code after take = %d, want 0", got)
	}
}

func TestSessionKeepaliveClosesBlockedSession(t *testing.T) {
	requested := make(chan struct{})
	closed := make(chan struct{})
	stop := startSessionKeepalive(func() error {
		close(requested)
		<-closed
		return errors.New("session closed")
	}, func() error {
		close(closed)
		return nil
	}, time.Millisecond, 10*time.Millisecond)
	defer stop()

	assert.Require(t, assert.True(t, receiveWithin(requested, time.Second)))
	assert.True(t, receiveWithin(closed, time.Second))
}

func receiveWithin(ch <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

// startSessionExitServer serves one session channel per connection and answers
// exec/shell with the given exit status; a nil status closes the channel without
// one, the way some network devices do.
func startSessionExitServer(t *testing.T, status *uint32) string {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveSessionExit(conn, config, status)
		}
	}()
	return listener.Addr().String()
}

func serveSessionExit(conn net.Conn, config *ssh.ServerConfig, status *uint32) {
	serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer serverConn.Close()
	go ssh.DiscardRequests(requests)

	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "only session channels")
			continue
		}
		channel, channelReqs, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer channel.Close()
			for request := range channelReqs {
				if request.Type != "exec" && request.Type != "shell" {
					_ = request.Reply(false, nil)
					continue
				}
				_ = request.Reply(true, nil)
				if status != nil {
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{*status}))
				}
				return
			}
		}()
	}
}

func runTestSSHCommand(t *testing.T, client *ssh.Client, command string) error {
	t.Helper()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	return session.Run(command)
}

func dialTestSSHClient(t *testing.T, addr string) *ssh.Client {
	t.Helper()
	client, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            "tester",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

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
	var dialed string
	restore := setSSHClientFactoriesForTest(
		func(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
			dialed = addr
			return &ssh.Client{}, nil
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
	if dialed != "10.0.0.8:22" {
		t.Fatalf("dialed = %q, want target", dialed)
	}
}

func TestNewSSHClientWithoutJumpUsesDefaultPort(t *testing.T) {
	var dialed string
	restore := setSSHClientFactoriesForTest(
		func(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
			dialed = addr
			return &ssh.Client{}, nil
		},
		newSSHClientConn,
	)
	defer restore()

	host := sshTestHost("target", "10.0.0.8")
	host.Port = 0
	client, err := newSSHClientForConnection(ResolvedConnection{Target: host}, sshClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if dialed != "10.0.0.8:22" {
		t.Fatalf("dialed = %q, want default port", dialed)
	}
}

func TestNewSSHClientWithJumpWrapsJumpConnectError(t *testing.T) {
	restore := setSSHClientFactoriesForTest(
		func(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
			if addr == "1.2.3.4:22" {
				return nil, errors.New("jump down")
			}
			return &ssh.Client{}, nil
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
		func(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
			return &ssh.Client{}, nil
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
		func(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
			return &ssh.Client{}, nil
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

func TestKnownHostKeyTypesExactAndPortHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		"10.0.0.8 ssh-ed25519 AAAA",
		"[10.0.0.8]:2222 ecdsa-sha2-nistp256 AAAA",
		"other.example.com ssh-rsa AAAA",
	}, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	types, err := knownHostKeyTypes(path, knownhosts.Normalize("10.0.0.8:22"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(types, ",") != ssh.KeyAlgoED25519 {
		t.Fatalf("types = %#v", types)
	}

	types, err = knownHostKeyTypes(path, knownhosts.Normalize("10.0.0.8:2222"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(types, ",") != ssh.KeyAlgoECDSA256 {
		t.Fatalf("types = %#v", types)
	}
}

func TestKnownHostKeyTypesHashedHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	normalized := knownhosts.Normalize("10.0.0.8:22")
	hashed := testHashedKnownHost("salt", normalized)
	if err := os.WriteFile(path, []byte(hashed+" "+ssh.KeyAlgoED25519+" AAAA\n"), 0600); err != nil {
		t.Fatal(err)
	}

	types, err := knownHostKeyTypes(path, normalized)
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 1 || types[0] != ssh.KeyAlgoED25519 {
		t.Fatalf("types = %#v", types)
	}
}

func TestKnownHostKeyTypesCertAuthorityAndRevokedMarkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		"@cert-authority 10.0.0.8 ssh-ed25519 AAAA",
		"@revoked 10.0.0.8 ecdsa-sha2-nistp256 AAAA",
	}, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	types, err := knownHostKeyTypes(path, knownhosts.Normalize("10.0.0.8:22"))
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 1 || types[0] != ssh.CertAlgoED25519v01 {
		t.Fatalf("types = %#v", types)
	}
}

func TestMergePreferredHostKeyAlgorithms(t *testing.T) {
	got := mergePreferredHostKeyAlgorithms([]string{ssh.KeyAlgoECDSA256, ssh.KeyAlgoED25519})
	if len(got) < 3 {
		t.Fatalf("algorithms = %#v", got)
	}
	if got[0] != ssh.KeyAlgoED25519 || got[1] != ssh.KeyAlgoECDSA256 {
		t.Fatalf("algorithms = %#v", got[:3])
	}
}

func TestMergePreferredHostKeyAlgorithmsMapsRSAKeyTypeToSHA2(t *testing.T) {
	got := mergePreferredHostKeyAlgorithms([]string{ssh.KeyAlgoRSA})
	want := []string{ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSA}
	if len(got) < len(want) {
		t.Fatalf("algorithms = %#v", got)
	}
	for i, value := range want {
		if got[i] != value {
			t.Fatalf("algorithms = %#v, want prefix %#v", got[:len(want)], want)
		}
	}
}

func TestMergePreferredHostKeyAlgorithmsKeepsCertificateFallbacks(t *testing.T) {
	got := mergePreferredHostKeyAlgorithms([]string{ssh.KeyAlgoED25519})
	if !testStringSliceContains(got, ssh.CertAlgoED25519v01) {
		t.Fatalf("algorithms missing cert fallback: %#v", got)
	}
}

func TestSSHClientConfigPrefersKnownHostKeyType(t *testing.T) {
	withTempConfig(t)
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := appendKnownHostKey(path, "10.0.0.8:22", testED25519PublicKey(t)); err != nil {
		t.Fatal(err)
	}

	config, err := sshClientConfig(Host{
		Name:           "devhost",
		IP:             "10.0.0.8",
		User:           "root",
		Password:       "secret",
		Port:           22,
		HostKeyCheck:   HostKeyCheckKnownHosts,
		KnownHostsPath: path,
	}, sshClientOptions{NoHostKeyPrompt: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(config.HostKeyAlgorithms) == 0 || config.HostKeyAlgorithms[0] != ssh.KeyAlgoED25519 {
		t.Fatalf("host key algorithms = %#v", config.HostKeyAlgorithms)
	}
}

func TestSSHClientConfigDoesNotSetPreferredAlgorithmsForInsecure(t *testing.T) {
	config, err := sshClientConfig(sshTestHost("devhost", "10.0.0.8"), sshClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(config.HostKeyAlgorithms) != 0 {
		t.Fatalf("host key algorithms = %#v", config.HostKeyAlgorithms)
	}
}

func setSSHClientFactoriesForTest(
	newDial func(string, string, *ssh.ClientConfig) (*ssh.Client, error),
	newClientConn func(net.Conn, string, *ssh.ClientConfig) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error),
) func() {
	oldNewSSHDial := newSSHDial
	oldNewSSHClientConn := newSSHClientConn
	newSSHDial = newDial
	newSSHClientConn = newClientConn
	return func() {
		newSSHDial = oldNewSSHDial
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

func testHashedKnownHost(salt, hostname string) string {
	mac := hmac.New(sha1.New, []byte(salt))
	_, _ = mac.Write([]byte(hostname))
	return "|1|" + base64.StdEncoding.EncodeToString([]byte(salt)) + "|" + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func testED25519PublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func testStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
