package core

import (
	"strings"
	"testing"
)

func TestParseHostImportIPs(t *testing.T) {
	hosts, errs := ParseHostImportIPs(strings.NewReader("10.0.0.8\nweb.internal\n"), HostImportDefaults{AuthRef: "dev-root", Group: "testing"})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	if len(hosts) != 2 {
		t.Fatalf("hosts len = %d", len(hosts))
	}
	if hosts[0].Name != "10.0.0.8" || hosts[0].IP != "10.0.0.8" || hosts[0].AuthRef != "dev-root" || hosts[0].Group != "testing" {
		t.Fatalf("host[0] = %+v", hosts[0])
	}
	if hosts[1].Name != "web.internal" || hosts[1].IP != "web.internal" {
		t.Fatalf("host[1] = %+v", hosts[1])
	}
}

func TestParseHostImportIPsIgnoresBlankAndCommentLines(t *testing.T) {
	input := "\n# comment\n10.0.0.8\n\n  # another comment\r\n10.0.0.9\n"
	hosts, errs := ParseHostImportIPs(strings.NewReader(input), HostImportDefaults{})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	if len(hosts) != 2 || hosts[0].IP != "10.0.0.8" || hosts[1].IP != "10.0.0.9" {
		t.Fatalf("hosts = %+v", hosts)
	}
}

func TestParseHostImportPlain(t *testing.T) {
	input := `ip=10.0.0.8
name=devhost
auth=dev-root
group=testing
remark=app server
port=2222
jump=bastion
host_key_check=insecure

ip: 10.0.0.9
name: dbhost
user: root
password: secret
key_path: ~/.ssh/id_ed25519
connect_timeout: 10s
run_timeout: 1m
remote_script_dir: /var/tmp
known_hosts_path: ~/.ssh/known_hosts
`
	hosts, errs := ParseHostImportPlain(strings.NewReader(input), HostImportDefaults{})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	if len(hosts) != 2 {
		t.Fatalf("hosts len = %d", len(hosts))
	}
	if hosts[0].Name != "devhost" || hosts[0].AuthRef != "dev-root" || hosts[0].Group != "testing" || hosts[0].Port != 2222 || hosts[0].Jump != "bastion" {
		t.Fatalf("host[0] = %+v", hosts[0])
	}
	if hosts[1].Name != "dbhost" || hosts[1].User != "root" || hosts[1].Password != "secret" || hosts[1].KeyPath != "~/.ssh/id_ed25519" {
		t.Fatalf("host[1] = %+v", hosts[1])
	}
	if hosts[1].ConnectTimeout != "10s" || hosts[1].RunTimeout != "1m" || hosts[1].RemoteScriptDir != "/var/tmp" || hosts[1].KnownHostsPath == "" {
		t.Fatalf("host[1] defaults = %+v", hosts[1])
	}
}

func TestParseHostImportPlainSeparatesHostsByBlankLine(t *testing.T) {
	input := "ip=10.0.0.8\n\n\nip=10.0.0.9\n"
	hosts, errs := ParseHostImportPlain(strings.NewReader(input), HostImportDefaults{})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	if len(hosts) != 2 || hosts[0].IP != "10.0.0.8" || hosts[1].IP != "10.0.0.9" {
		t.Fatalf("hosts = %+v", hosts)
	}
}

func TestParseHostImportPlainSupportsAliases(t *testing.T) {
	input := `hostname=10.0.0.8
username=root
pwd=secret
keypath=~/.ssh/id_rsa
jump_host=bastion
`
	hosts, errs := ParseHostImportPlain(strings.NewReader(input), HostImportDefaults{})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	host := hosts[0]
	if host.IP != "10.0.0.8" || host.User != "root" || host.Password != "secret" || host.KeyPath != "~/.ssh/id_rsa" || host.Jump != "bastion" {
		t.Fatalf("host = %+v", host)
	}
}

func TestParseHostImportCSV(t *testing.T) {
	input := "name,ip,auth,group,remark,port,jump\n" +
		"devhost,10.0.0.8,dev-root,testing,app server,2222,bastion\n" +
		"dbhost,10.0.0.9,dev-root,testing,db server,22,bastion\n"
	hosts, errs := ParseHostImportCSV(strings.NewReader(input), HostImportDefaults{})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	if len(hosts) != 2 {
		t.Fatalf("hosts len = %d", len(hosts))
	}
	if hosts[0].Name != "devhost" || hosts[0].IP != "10.0.0.8" || hosts[0].AuthRef != "dev-root" || hosts[0].Port != 2222 {
		t.Fatalf("host[0] = %+v", hosts[0])
	}
}

func TestParseHostImportCSVSupportsAliases(t *testing.T) {
	input := "hostname,username,pwd,key,auth_ref\n10.0.0.8,root,secret,~/.ssh/id_rsa,ops\n"
	hosts, errs := ParseHostImportCSV(strings.NewReader(input), HostImportDefaults{})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	host := hosts[0]
	if host.IP != "10.0.0.8" || host.User != "root" || host.Password != "secret" || host.KeyPath != "~/.ssh/id_rsa" || host.AuthRef != "ops" {
		t.Fatalf("host = %+v", host)
	}
}

func TestParseHostImportCSVKeepsCommaInRemark(t *testing.T) {
	input := "name,ip,remark\nweb,10.0.0.8,\"app, server\"\n"
	hosts, errs := ParseHostImportCSV(strings.NewReader(input), HostImportDefaults{})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	if hosts[0].Remark != "app, server" {
		t.Fatalf("remark = %q", hosts[0].Remark)
	}
}

func TestParseHostImportAppliesDefaults(t *testing.T) {
	hosts, errs := ParseHostImport(strings.NewReader("10.0.0.8\n"), HostImportIPs, HostImportDefaults{
		AuthRef:         "dev-root",
		User:            "root",
		KeyPath:         "~/.ssh/id_rsa",
		Group:           "testing",
		Remark:          "imported",
		Port:            2222,
		Jump:            "bastion",
		ConnectTimeout:  "10s",
		RunTimeout:      "1m",
		RemoteScriptDir: "/var/tmp",
		HostKeyCheck:    HostKeyCheckInsecure,
		KnownHostsPath:  "~/.ssh/known_hosts",
	})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	host := hosts[0]
	if host.AuthRef != "dev-root" || host.User != "root" || host.KeyPath != "~/.ssh/id_rsa" || host.Group != "testing" || host.Port != 2222 {
		t.Fatalf("host = %+v", host)
	}
	if host.Jump != "bastion" || host.ConnectTimeout != "10s" || host.RunTimeout != "1m" || host.RemoteScriptDir != "/var/tmp" {
		t.Fatalf("host defaults = %+v", host)
	}
	if host.HostKeyCheck != HostKeyCheckInsecure || host.KnownHostsPath != "~/.ssh/known_hosts" {
		t.Fatalf("host key defaults = %+v", host)
	}
}

func TestParseHostImportRowOverridesDefaults(t *testing.T) {
	input := "ip=10.0.0.8\nname=devhost\nauth=host-auth\ngroup=host-group\nport=2200\n"
	hosts, errs := ParseHostImportPlain(strings.NewReader(input), HostImportDefaults{AuthRef: "default-auth", Group: "default-group", Port: 22})
	if len(errs) != 0 {
		t.Fatalf("errs = %+v", errs)
	}
	if hosts[0].AuthRef != "host-auth" || hosts[0].Group != "host-group" || hosts[0].Port != 2200 {
		t.Fatalf("host = %+v", hosts[0])
	}
}

func TestParseHostImportRejectsUnknownField(t *testing.T) {
	_, errs := ParseHostImportPlain(strings.NewReader("ip=10.0.0.8\nbad=value\n"), HostImportDefaults{})
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "unknown field") {
		t.Fatalf("errs = %+v", errs)
	}

	_, errs = ParseHostImportCSV(strings.NewReader("ip,bad\n10.0.0.8,value\n"), HostImportDefaults{})
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "unknown csv header field") {
		t.Fatalf("csv errs = %+v", errs)
	}
}

func TestParseHostImportRejectsInvalidPort(t *testing.T) {
	_, errs := ParseHostImportCSV(strings.NewReader("ip,port\n10.0.0.8,70000\n"), HostImportDefaults{})
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "invalid ssh port") {
		t.Fatalf("errs = %+v", errs)
	}
}

func TestParseHostImportRejectsInvalidHostKeyCheck(t *testing.T) {
	_, errs := ParseHostImportPlain(strings.NewReader("ip=10.0.0.8\nhost_key_check=bad\n"), HostImportDefaults{})
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "invalid host_key_check") {
		t.Fatalf("errs = %+v", errs)
	}
}
