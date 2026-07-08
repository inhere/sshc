package core

import (
	"strings"
	"testing"
)

func TestParseSSHConfigImport(t *testing.T) {
	input := `
Host *
  User ignored

Host devhost
  HostName 10.0.0.8
  User root
  Port 2222
  IdentityFile ~/.ssh/id_rsa
  ProxyJump bastion

Host *.internal
  HostName ignored
`
	hosts, warnings, errs := ParseSSHConfigImport(strings.NewReader(input), SSHConfigImportOptions{Defaults: HostImportDefaults{Group: "imported", Tags: []string{"ssh-config"}}})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v", warnings)
	}
	if len(hosts) != 1 {
		t.Fatalf("hosts = %+v", hosts)
	}
	host := hosts[0]
	if host.Name != "devhost" || host.IP != "10.0.0.8" || host.User != "root" || host.Port != 2222 || host.KeyPath != "~/.ssh/id_rsa" || host.Jump != "bastion" {
		t.Fatalf("host = %+v", host)
	}
	if host.Group != "imported" || strings.Join(host.Tags, ",") != "ssh-config" {
		t.Fatalf("host defaults = %+v", host)
	}
}

func TestParseSSHConfigImportAuthSkipsUserAndIdentityByDefault(t *testing.T) {
	input := `
Host devhost
  HostName 10.0.0.8
  User root
  IdentityFile ~/.ssh/id_rsa
`
	hosts, _, errs := ParseSSHConfigImport(strings.NewReader(input), SSHConfigImportOptions{Defaults: HostImportDefaults{AuthRef: "dev-root", Port: 22}})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	host := hosts[0]
	if host.AuthRef != "dev-root" || host.User != "" || host.KeyPath != "" {
		t.Fatalf("host = %+v", host)
	}
}

func TestParseSSHConfigImportIdentityFileWithAuth(t *testing.T) {
	input := `
Host devhost
  HostName 10.0.0.8
  User root
  IdentityFile ~/.ssh/id_rsa
`
	hosts, _, errs := ParseSSHConfigImport(strings.NewReader(input), SSHConfigImportOptions{
		Defaults:           HostImportDefaults{AuthRef: "dev-root", Port: 22},
		ImportIdentityFile: true,
	})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	host := hosts[0]
	if host.AuthRef != "dev-root" || host.User != "" || host.KeyPath != "~/.ssh/id_rsa" {
		t.Fatalf("host = %+v", host)
	}
}

func TestParseSSHConfigImportWarnings(t *testing.T) {
	input := `
Include ~/.ssh/conf.d/*
Host devhost
  HostName 10.0.0.8
  User root
  IdentityFile ~/.ssh/id_rsa
  IdentityFile ~/.ssh/second
  ProxyJump bastion,inner
  ProxyCommand ssh bastion nc %h %p
  LocalForward 8080 localhost:80
`
	hosts, warnings, errs := ParseSSHConfigImport(strings.NewReader(input), SSHConfigImportOptions{Defaults: HostImportDefaults{Port: 22}})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	if len(hosts) != 1 {
		t.Fatalf("hosts = %+v", hosts)
	}
	if len(warnings) < 4 {
		t.Fatalf("warnings = %+v", warnings)
	}
	if hosts[0].Jump != "" {
		t.Fatalf("jump = %q", hosts[0].Jump)
	}
}
