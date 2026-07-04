package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadHostsFileIgnoresBlankAndCommentLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts.txt")
	data := "# comment\n\n devhost \n192.168.1.10\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	hosts, err := ReadHostsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[0] != "devhost" || hosts[1] != "192.168.1.10" {
		t.Fatalf("hosts = %#v", hosts)
	}
}

func TestResolveBatchHostsFromCommaList(t *testing.T) {
	withTempConfig(t)
	if err := SaveConfig(&Config{Hosts: []Host{
		{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22},
		{Name: "web-2", IP: "10.0.0.9", User: "root", Password: "secret", Port: 22},
	}}); err != nil {
		t.Fatal(err)
	}

	hosts, err := ResolveBatchHosts(BatchHostSource{Hosts: []string{"devhost,web-2"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[0].Name != "devhost" || hosts[1].Name != "web-2" {
		t.Fatalf("hosts = %+v", hosts)
	}
}

func TestResolveBatchHostsRejectsMultipleSources(t *testing.T) {
	withTempConfig(t)
	_, err := ResolveBatchHosts(BatchHostSource{Hosts: []string{"devhost"}, Group: "testing"})
	if err == nil {
		t.Fatal("expected multiple sources error")
	}
}

func TestResolveBatchHostsFromGroup(t *testing.T) {
	withTempConfig(t)
	if err := SaveConfig(&Config{Hosts: []Host{
		{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22, Group: "testing"},
		{Name: "web-2", IP: "10.0.0.9", User: "root", Password: "secret", Port: 22, Group: "prod"},
	}}); err != nil {
		t.Fatal(err)
	}

	hosts, err := ResolveBatchHosts(BatchHostSource{Group: "testing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Name != "devhost" {
		t.Fatalf("hosts = %+v", hosts)
	}
}

func TestResolveBatchHostsDeduplicates(t *testing.T) {
	withTempConfig(t)
	if err := SaveConfig(&Config{Hosts: []Host{
		{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22},
	}}); err != nil {
		t.Fatal(err)
	}

	hosts, err := ResolveBatchHosts(BatchHostSource{Hosts: []string{"devhost,10.0.0.8"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Name != "devhost" {
		t.Fatalf("hosts = %+v", hosts)
	}
}

func TestResolveBatchHostsRejectsMissingHost(t *testing.T) {
	withTempConfig(t)
	if err := SaveConfig(&Config{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveBatchHosts(BatchHostSource{Hosts: []string{"missing"}}); err == nil {
		t.Fatal("expected missing host error")
	}
}

func TestResolveBatchHostsFileRawIPsWithAuthRef(t *testing.T) {
	withTempConfig(t)
	path := filepath.Join(t.TempDir(), "ips.txt")
	if err := os.WriteFile(path, []byte("10.0.0.8\n10.0.0.9\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(&Config{
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root", Password: "secret"}},
	}); err != nil {
		t.Fatal(err)
	}

	hosts, err := ResolveBatchHosts(BatchHostSource{HostsFile: path, AuthRef: "dev-root", AllowRaw: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[0].IP != "10.0.0.8" || hosts[0].User != "root" || hosts[0].Password != "secret" {
		t.Fatalf("hosts = %+v", hosts)
	}
}

func TestResolveBatchHostsRawIPDoesNotPersist(t *testing.T) {
	withTempConfig(t)
	if err := SaveConfig(&Config{
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root", Password: "secret"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveBatchHosts(BatchHostSource{Hosts: []string{"10.0.0.8"}, AuthRef: "dev-root", AllowRaw: true}); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Hosts) != 0 {
		t.Fatalf("raw host persisted: %+v", config.Hosts)
	}
}

func TestResolveBatchHostsRejectsRawIPWithoutAuth(t *testing.T) {
	withTempConfig(t)
	if err := SaveConfig(&Config{Defaults: Defaults{User: "root", Port: 22}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveBatchHosts(BatchHostSource{Hosts: []string{"10.0.0.8"}, AllowRaw: true}); err == nil {
		t.Fatal("expected raw host without auth error")
	}
}

func TestResolveBatchHostsUsesSavedHostBeforeRaw(t *testing.T) {
	withTempConfig(t)
	if err := SaveConfig(&Config{
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root", Password: "secret"}},
		Hosts:        []Host{{Name: "10.0.0.8", IP: "10.0.0.9", AuthRef: "dev-root", Port: 22}},
	}); err != nil {
		t.Fatal(err)
	}

	hosts, err := ResolveBatchHosts(BatchHostSource{Hosts: []string{"10.0.0.8"}, AuthRef: "dev-root", AllowRaw: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].IP != "10.0.0.9" {
		t.Fatalf("saved host was not preferred: %+v", hosts)
	}
}

func TestResolveBatchHostsRawIPUsesPortOverride(t *testing.T) {
	withTempConfig(t)
	if err := SaveConfig(&Config{
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root", Password: "secret"}},
	}); err != nil {
		t.Fatal(err)
	}

	hosts, err := ResolveBatchHosts(BatchHostSource{
		Hosts:     []string{"10.0.0.8"},
		AuthRef:   "dev-root",
		AllowRaw:  true,
		Overrides: HostOverrides{Port: 2222},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Port != 2222 {
		t.Fatalf("hosts = %+v", hosts)
	}
}

func TestResolveBatchHostsRejectsHostPortRawTarget(t *testing.T) {
	withTempConfig(t)
	if err := SaveConfig(&Config{
		AuthProfiles: []AuthProfile{{Name: "dev-root", User: "root", Password: "secret"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveBatchHosts(BatchHostSource{Hosts: []string{"10.0.0.8:22"}, AuthRef: "dev-root", AllowRaw: true}); err == nil {
		t.Fatal("expected host:port raw target error")
	}
}
