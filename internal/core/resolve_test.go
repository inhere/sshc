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
