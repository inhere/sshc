package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSSHConfig(t *testing.T) {
	config := `
Host *
  User ignored

Host devhost
  HostName 10.0.0.8
  User root
  Port 2222
  IdentityFile ~/.ssh/id_rsa

Host *.internal
  HostName ignored
  User ignored
  IdentityFile ~/.ssh/ignored
`
	hosts, err := ParseSSHConfig(strings.NewReader(config))
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 {
		t.Fatalf("hosts len = %d, want 1: %+v", len(hosts), hosts)
	}
	host := hosts[0]
	if host.Name != "devhost" || host.IP != "10.0.0.8" || host.User != "root" || host.Port != 2222 || host.KeyPath != "~/.ssh/id_rsa" {
		t.Fatalf("host = %+v", host)
	}
	if host.Group != "ssh-config" {
		t.Fatalf("group = %q", host.Group)
	}
}

func TestLoadStoreWithSSHConfigKeepsExplicitHost(t *testing.T) {
	withTempConfig(t)
	home, err := userHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	config := "Host devhost\n  HostName 10.0.0.9\n  User ops\n  IdentityFile ~/.ssh/id_rsa\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveStore(&Store{Hosts: []Host{{
		Name:     "devhost",
		IP:       "10.0.0.8",
		User:     "root",
		Password: "secret",
		Port:     22,
	}}}); err != nil {
		t.Fatal(err)
	}

	store, err := LoadStoreWithSSHConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Hosts) != 1 {
		t.Fatalf("hosts len = %d, want 1", len(store.Hosts))
	}
	if store.Hosts[0].IP != "10.0.0.8" {
		t.Fatalf("explicit host was not preserved: %+v", store.Hosts[0])
	}
}
