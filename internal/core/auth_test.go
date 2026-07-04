package core

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	_, err := hostKeyCallback(Host{HostKeyCheck: HostKeyCheckKnownHosts, KnownHostsPath: filepath.Join(t.TempDir(), "missing_known_hosts")})
	if err == nil || !strings.Contains(err.Error(), "load known_hosts") {
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
