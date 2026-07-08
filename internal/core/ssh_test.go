package core

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
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
