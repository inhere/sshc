package core

import (
	"strings"
	"testing"
)

func TestStoreResolveHostUsesExactMatchFirst(t *testing.T) {
	store := Store{Hosts: []Host{
		{Name: "dev", IP: "10.0.0.8", User: "root", Password: "one", Port: 22},
		{Name: "devhost", IP: "10.0.0.9", User: "root", Password: "two", Port: 22},
	}}

	host, ok, err := store.ResolveHost("dev")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || host.Name != "dev" {
		t.Fatalf("host = %+v, ok = %v", host, ok)
	}
}

func TestStoreResolveHostMatchesUniqueParts(t *testing.T) {
	store := Store{Hosts: []Host{
		{Name: "testing-web", IP: "10.0.0.8", User: "root", Password: "one", Port: 22},
		{Name: "testing-db", IP: "10.0.0.9", User: "root", Password: "two", Port: 22},
	}}

	host, ok, err := store.ResolveHost("test web")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || host.Name != "testing-web" {
		t.Fatalf("host = %+v, ok = %v", host, ok)
	}
}

func TestStoreResolveHostMatchesRemarkAndGroup(t *testing.T) {
	store := Store{Hosts: []Host{
		{Name: "web-a", IP: "10.0.0.8", User: "root", Password: "one", Remark: "gpu runner", Group: "testing", Port: 22},
		{Name: "web-b", IP: "10.0.0.9", User: "root", Password: "two", Remark: "api server", Group: "prod", Port: 22},
	}}

	host, ok, err := store.ResolveHost("testing gpu")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || host.Name != "web-a" {
		t.Fatalf("host = %+v, ok = %v", host, ok)
	}
}

func TestStoreResolveHostRejectsMultiplePartialMatches(t *testing.T) {
	store := Store{Hosts: []Host{
		{Name: "testing-web", IP: "10.0.0.8", User: "root", Password: "one", Port: 22},
		{Name: "testing-db", IP: "10.0.0.9", User: "root", Password: "two", Port: 22},
	}}

	_, ok, err := store.ResolveHost("testing")
	if err == nil {
		t.Fatal("expected multiple match error")
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
	for _, want := range []string{"testing-web", "testing-db"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err %q does not contain %q", err.Error(), want)
		}
	}
}

func TestResolveEffectiveHostWithAuthUsesRawAddressAndDoesNotPersist(t *testing.T) {
	config := Config{AuthProfiles: []AuthProfile{{Name: "ops", User: "root", Password: "secret"}}}
	host, ok, err := config.ResolveEffectiveHostWithAuth("192.0.2.10", "ops")
	if err != nil || !ok {
		t.Fatalf("resolve: host=%+v ok=%v err=%v", host, ok, err)
	}
	if host.IP != "192.0.2.10" || host.User != "root" || host.Password != "secret" || host.Port != 22 {
		t.Fatalf("host=%+v", host)
	}
	if len(config.Hosts) != 0 {
		t.Fatalf("raw target was persisted: %+v", config.Hosts)
	}
	if _, _, err := config.ResolveEffectiveHostWithAuth("192.0.2.10:22", "ops"); err == nil {
		t.Fatal("expected host:port rejection")
	}
	if _, _, err := config.ResolveEffectiveHostWithAuth("192.0.2.10", "missing"); err == nil {
		t.Fatal("expected missing auth profile error")
	}
}

func TestResolveEffectiveHostWithAuthOverridesSavedCredentials(t *testing.T) {
	config := Config{
		AuthProfiles: []AuthProfile{{Name: "ops", User: "deploy", KeyPath: "~/.ssh/ops"}},
		Groups:       map[string]GroupDefaults{"team": {User: "group-user", KeyPath: "~/.ssh/group"}},
		Hosts:        []Host{{Name: "saved", IP: "192.0.2.11", User: "old", Password: "old-secret", Port: 2222, Jump: "bastion", Group: "team"}},
	}
	host, ok, err := config.ResolveEffectiveHostWithAuth("saved", "ops")
	if err != nil || !ok {
		t.Fatalf("resolve: host=%+v ok=%v err=%v", host, ok, err)
	}
	if host.IP != "192.0.2.11" || host.Port != 2222 || host.Jump != "bastion" || host.User != "deploy" || host.KeyPath != "~/.ssh/ops" || host.Password != "" {
		t.Fatalf("host=%+v", host)
	}
}
