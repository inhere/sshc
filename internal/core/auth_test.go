package core

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestHostAuthUsesKeyAndPassword(t *testing.T) {
	keyPath := writeTestPrivateKey(t)
	auth, err := hostAuth(Host{KeyPath: keyPath, Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if len(auth) != 3 {
		t.Fatalf("auth methods = %d, want 3", len(auth))
	}
}

func TestResolveEffectiveHostFromInlineAuth(t *testing.T) {
	config := Config{Hosts: []Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     2222,
	}}}

	host, ok, err := config.ResolveEffectiveHost("devhost", HostOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || host.User != "root" || host.Password != "secret" || host.Port != 2222 {
		t.Fatalf("host = %+v, ok = %v", host, ok)
	}
	if host.HostKeyCheck != HostKeyCheckKnownHosts || host.KnownHostsPath != DefaultKnownHostsPath {
		t.Fatalf("host key settings = %+v", host)
	}
}

func TestResolveEffectiveHostFromAuthProfile(t *testing.T) {
	config := Config{
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root", Password: "secret", KeyPath: "~/.ssh/id_rsa"}},
		Hosts:        []Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "dev-root", Port: 22}},
	}

	host, ok, err := config.ResolveEffectiveHost("devhost", HostOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || host.User != "root" || host.Password != "secret" || host.KeyPath != "~/.ssh/id_rsa" {
		t.Fatalf("host = %+v, ok = %v", host, ok)
	}
}

func TestResolveEffectiveHostHostOverridesAuthProfile(t *testing.T) {
	config := Config{
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root", Password: "profile", KeyPath: "~/.ssh/profile"}},
		Hosts: []Host{{
			Name:     "devhost",
			IP:       "10.0.0.8",
			AuthRef:  "dev-root",
			User:     "ops",
			Password: "host",
			KeyPath:  "~/.ssh/host",
			Port:     2222,
		}},
	}

	host, ok, err := config.ResolveEffectiveHost("devhost", HostOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || host.User != "ops" || host.Password != "host" || host.KeyPath != "~/.ssh/host" || host.Port != 2222 {
		t.Fatalf("host = %+v, ok = %v", host, ok)
	}
}

func TestResolveEffectiveHostUsesDefaults(t *testing.T) {
	config := Config{
		Defaults: Defaults{
			User:            "root",
			Port:            2200,
			ConnectTimeout:  "15s",
			RemoteScriptDir: "/opt/tmp",
			HostKeyCheck:    HostKeyCheckInsecure,
			KnownHostsPath:  "~/custom_known_hosts",
		},
		Hosts: []Host{{Name: "devhost", IP: "10.0.0.8", KeyPath: "~/.ssh/id_rsa"}},
	}

	host, ok, err := config.ResolveEffectiveHost("devhost", HostOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || host.User != "root" || host.Port != 2200 || host.ConnectTimeout != "15s" || host.RemoteScriptDir != "/opt/tmp" {
		t.Fatalf("host = %+v, ok = %v", host, ok)
	}
	if host.HostKeyCheck != HostKeyCheckInsecure || host.KnownHostsPath != "~/custom_known_hosts" {
		t.Fatalf("host key settings = %+v", host)
	}
}

func TestResolveEffectiveHostUsesGroupDefaults(t *testing.T) {
	config := Config{
		Groups: map[string]GroupDefaults{
			"testing": {
				AuthRef:         "dev-root",
				Port:            2222,
				Jump:            "bastion",
				ConnectTimeout:  "10s",
				RunTimeout:      "1m",
				RemoteScriptDir: "/var/tmp",
				HostKeyCheck:    HostKeyCheckInsecure,
				KnownHostsPath:  "~/.ssh/group_known_hosts",
			},
		},
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}},
		Hosts: []Host{
			{Name: "bastion", IP: "1.2.3.4", AuthRef: "dev-root"},
			{Name: "devhost", IP: "10.0.0.8", Group: "testing"},
		},
	}

	host, ok, err := config.ResolveEffectiveHost("devhost", HostOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("host not found")
	}
	if host.AuthRef != "dev-root" || host.User != "root" || host.KeyPath != "~/.ssh/id_rsa" || host.Port != 2222 || host.Jump != "bastion" {
		t.Fatalf("host = %+v", host)
	}
	if host.ConnectTimeout != "10s" || host.RunTimeout != "1m" || host.RemoteScriptDir != "/var/tmp" {
		t.Fatalf("timeouts = %+v", host)
	}
	if host.HostKeyCheck != HostKeyCheckInsecure || host.KnownHostsPath != "~/.ssh/group_known_hosts" {
		t.Fatalf("host key defaults = %+v", host)
	}
}

func TestResolveEffectiveHostHostOverridesGroupDefaults(t *testing.T) {
	config := Config{
		Groups:       map[string]GroupDefaults{"testing": {AuthRef: "dev-root", User: "group-user", KeyPath: "~/.ssh/group", Port: 2222, Jump: "bastion"}},
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}},
		Hosts: []Host{
			{Name: "bastion", IP: "1.2.3.4", AuthRef: "dev-root"},
			{Name: "devhost", IP: "10.0.0.8", Group: "testing", User: "deploy", KeyPath: "~/.ssh/deploy", Port: 2200, Jump: "alt-bastion"},
		},
	}

	host, ok, err := config.ResolveEffectiveHost("devhost", HostOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("host not found")
	}
	if host.User != "deploy" || host.KeyPath != "~/.ssh/deploy" || host.Port != 2200 || host.Jump != "alt-bastion" {
		t.Fatalf("host = %+v", host)
	}
}

func TestResolveEffectiveHostHostAuthRefOverridesGroupAuthRef(t *testing.T) {
	config := Config{
		Groups: map[string]GroupDefaults{"testing": {AuthRef: "group-auth"}},
		AuthProfiles: []AuthProfile{
			{Name: "group-auth", User: "group", KeyPath: "~/.ssh/group"},
			{Name: "host-auth", User: "host", KeyPath: "~/.ssh/host"},
		},
		Hosts: []Host{{Name: "devhost", IP: "10.0.0.8", Group: "testing", AuthRef: "host-auth"}},
	}

	host, ok, err := config.ResolveEffectiveHost("devhost", HostOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("host not found")
	}
	if host.AuthRef != "host-auth" || host.User != "host" || host.KeyPath != "~/.ssh/host" {
		t.Fatalf("host = %+v", host)
	}
}

func TestResolveEffectiveHostMissingAuthRef(t *testing.T) {
	config := Config{Hosts: []Host{{Name: "devhost", IP: "10.0.0.8", AuthRef: "missing", Port: 22}}}
	_, ok, err := config.ResolveEffectiveHost("devhost", HostOverrides{})
	if err == nil || !strings.Contains(err.Error(), `auth profile "missing" not found`) {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
}

func TestResolveEffectiveHostRequiresAuthMethod(t *testing.T) {
	config := Config{Defaults: Defaults{User: "root", Port: 22}, Hosts: []Host{{Name: "devhost", IP: "10.0.0.8"}}}
	_, ok, err := config.ResolveEffectiveHost("devhost", HostOverrides{})
	if err == nil || !strings.Contains(err.Error(), "password or key_path") {
		t.Fatalf("err = %v", err)
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
}

func TestHostKeyCallbackUsesInsecureOnlyWhenConfigured(t *testing.T) {
	if _, err := hostKeyCallback(Host{HostKeyCheck: HostKeyCheckInsecure}); err != nil {
		t.Fatalf("insecure callback: %v", err)
	}

	path := filepath.Join(t.TempDir(), "missing_known_hosts")
	if _, err := hostKeyCallback(Host{HostKeyCheck: HostKeyCheckKnownHosts, KnownHostsPath: path}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("known_hosts file not created: %v", err)
	}
}

func TestHostKeyCallbackPromptsAndAppendsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	key := testPublicKey(t)
	t.Cleanup(setConfirmUnknownHostKeyForTest(func(info UnknownHostKey) (bool, error) {
		if info.Hostname != "example.com:22" || info.KnownHostsPath != path {
			t.Fatalf("info = %+v", info)
		}
		return true, nil
	}))

	callback, err := hostKeyCallback(Host{HostKeyCheck: HostKeyCheckKnownHosts, KnownHostsPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("example.com:22", testRemoteAddr(), key); err != nil {
		t.Fatalf("callback: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "example.com") {
		t.Fatalf("known_hosts content = %q", string(content))
	}

	callback, err = hostKeyCallback(Host{HostKeyCheck: HostKeyCheckKnownHosts, KnownHostsPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("example.com:22", testRemoteAddr(), key); err != nil {
		t.Fatalf("trusted callback: %v", err)
	}
}

func TestHostKeyCallbackRejectsUnknownKeyWhenNotConfirmed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	t.Cleanup(setConfirmUnknownHostKeyForTest(func(info UnknownHostKey) (bool, error) {
		return false, nil
	}))

	callback, err := hostKeyCallback(Host{HostKeyCheck: HostKeyCheckKnownHosts, KnownHostsPath: path})
	if err != nil {
		t.Fatal(err)
	}
	err = callback("example.com:22", testRemoteAddr(), testPublicKey(t))
	if err == nil || !strings.Contains(err.Error(), "host key for example.com:22 is unknown") {
		t.Fatalf("err = %v", err)
	}
}

func TestHostKeyCallbackDoesNotPromptOnChangedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	firstKey := testPublicKey(t)
	secondKey := testPublicKey(t)
	if err := appendKnownHostKey(path, "example.com:22", firstKey); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(setConfirmUnknownHostKeyForTest(func(info UnknownHostKey) (bool, error) {
		t.Fatal("changed host key should not prompt")
		return false, nil
	}))

	callback, err := hostKeyCallback(Host{HostKeyCheck: HostKeyCheckKnownHosts, KnownHostsPath: path})
	if err != nil {
		t.Fatal(err)
	}
	err = callback("example.com:22", testRemoteAddr(), secondKey)
	if err == nil {
		t.Fatal("expected changed host key error")
	}
}

func TestTrustHostKeyAddsUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	key := testPublicKey(t)
	t.Cleanup(setScanSSHHostKeyForTest(func(host Host) (ssh.PublicKey, net.Addr, error) {
		if host.IP != "example.com" || host.Port != 2222 {
			t.Fatalf("host = %+v", host)
		}
		return key, testRemoteAddr(), nil
	}))

	result, err := TrustHostKey(Host{Name: "devhost", IP: "example.com", Port: 2222, KnownHostsPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "added" || result.Address != "example.com:2222" || result.Fingerprint != ssh.FingerprintSHA256(key) {
		t.Fatalf("result = %+v", result)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "[example.com]:2222") {
		t.Fatalf("known_hosts content = %q", string(content))
	}
}

func TestTrustHostKeyAlreadyTrusted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	key := testPublicKey(t)
	if err := appendKnownHostKey(path, "example.com:22", key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(setScanSSHHostKeyForTest(func(host Host) (ssh.PublicKey, net.Addr, error) {
		return key, testRemoteAddr(), nil
	}))

	result, err := TrustHostKey(Host{Name: "devhost", IP: "example.com", Port: 22, KnownHostsPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "already_trusted" {
		t.Fatalf("result = %+v", result)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(content), "example.com") != 1 {
		t.Fatalf("known_hosts content = %q", string(content))
	}
}

func TestTrustHostKeyRejectsChangedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := appendKnownHostKey(path, "example.com:22", testPublicKey(t)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(setScanSSHHostKeyForTest(func(host Host) (ssh.PublicKey, net.Addr, error) {
		return testPublicKey(t), testRemoteAddr(), nil
	}))

	_, err := TrustHostKey(Host{Name: "devhost", IP: "example.com", Port: 22, KnownHostsPath: path})
	if err == nil || !strings.Contains(err.Error(), "has changed") {
		t.Fatalf("err = %v", err)
	}
}

func TestTrustHostKeyForceReplacesChangedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	oldKey := testPublicKey(t)
	newKey := testPublicKey(t)
	if err := appendKnownHostKey(path, "example.com:22", oldKey); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(setScanSSHHostKeyForTest(func(host Host) (ssh.PublicKey, net.Addr, error) {
		return newKey, testRemoteAddr(), nil
	}))

	result, err := TrustHostKeyWithOptions(Host{Name: "devhost", IP: "example.com", Port: 22, KnownHostsPath: path}, HostKeyTrustOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "replaced" || result.Fingerprint != ssh.FingerprintSHA256(newKey) {
		t.Fatalf("result = %+v", result)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	oldEncoded := base64.StdEncoding.EncodeToString(oldKey.Marshal())
	if strings.Contains(string(content), oldEncoded) {
		t.Fatalf("known_hosts still contains old key: %q", string(content))
	}
	if strings.Count(string(content), "example.com") != 1 || !strings.Contains(string(content), newKey.Type()) {
		t.Fatalf("known_hosts content = %q", string(content))
	}
}

func TestTrustHostKeyRejectsCommandProxyHost(t *testing.T) {
	_, err := TrustHostKey(Host{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "pve-host"})
	if err == nil || !strings.Contains(err.Error(), "trust the via host instead") {
		t.Fatalf("err = %v", err)
	}
}

func writeTestPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	path := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return publicKey
}

func testRemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}
}

func setConfirmUnknownHostKeyForTest(fn func(UnknownHostKey) (bool, error)) func() {
	old := confirmUnknownHostKey
	confirmUnknownHostKey = fn
	return func() { confirmUnknownHostKey = old }
}

func setScanSSHHostKeyForTest(fn func(Host) (ssh.PublicKey, net.Addr, error)) func() {
	old := scanSSHHostKey
	scanSSHHostKey = fn
	return func() { scanSSHHostKey = old }
}
