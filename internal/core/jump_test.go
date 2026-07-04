package core

import (
	"strings"
	"testing"
)

func TestResolveConnectionWithoutJump(t *testing.T) {
	config := jumpTestConfig()

	host, ok, err := config.ResolveEffectiveHost("inner-db", HostOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("host not found")
	}
	host.Host.Jump = ""

	conn, err := config.ResolveConnection(host.ToHost())
	if err != nil {
		t.Fatal(err)
	}
	if conn.Target.Name != "inner-db" || conn.Jump != nil {
		t.Fatalf("connection = %+v", conn)
	}
}

func TestResolveConnectionFromHostJump(t *testing.T) {
	config := jumpTestConfig()

	host, ok, err := config.ResolveEffectiveHost("inner-db", HostOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("host not found")
	}
	conn, err := config.ResolveConnection(host.ToHost())
	if err != nil {
		t.Fatal(err)
	}
	if conn.Jump == nil || conn.Jump.Name != "bastion" || conn.Jump.IP != "1.2.3.4" {
		t.Fatalf("jump = %+v", conn.Jump)
	}
}

func TestResolveConnectionWithJumpOverride(t *testing.T) {
	withTempConfig(t)
	config := jumpTestConfig()
	config.Hosts = append(config.Hosts, Host{Name: "alt-bastion", IP: "1.2.3.5", AuthRef: "ops"})
	if err := SaveConfig(&config); err != nil {
		t.Fatal(err)
	}

	conn, ok, err := ResolveConnectionWithSSHConfig("inner-db", ResolveConnectionOptions{Jump: "alt-bastion"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("connection not found")
	}
	if conn.Target.Jump != "alt-bastion" {
		t.Fatalf("target jump = %q, want alt-bastion", conn.Target.Jump)
	}
	if conn.Jump == nil || conn.Jump.Name != "alt-bastion" {
		t.Fatalf("jump = %+v", conn.Jump)
	}
}

func TestResolveConnectionRejectsSelfJump(t *testing.T) {
	config := jumpTestConfig()
	host, ok, err := config.ResolveEffectiveHost("inner-db", HostOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("host not found")
	}
	raw := host.ToHost()
	raw.Jump = "inner-db"

	_, err = config.ResolveConnection(raw)
	if err == nil || !strings.Contains(err.Error(), "cannot jump through itself") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveConnectionRejectsNestedJump(t *testing.T) {
	config := jumpTestConfig()
	config.Hosts[0].Jump = "outer"
	config.Hosts = append(config.Hosts, Host{Name: "outer", IP: "1.2.3.6", AuthRef: "ops"})

	host, ok, err := config.ResolveEffectiveHost("inner-db", HostOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("host not found")
	}
	_, err = config.ResolveConnection(host.ToHost())
	if err == nil || !strings.Contains(err.Error(), "multi-level jump is not supported") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveConnectionMissingJumpHost(t *testing.T) {
	config := jumpTestConfig()
	host, ok, err := config.ResolveEffectiveHost("inner-db", HostOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("host not found")
	}
	raw := host.ToHost()
	raw.Jump = "missing"

	_, err = config.ResolveConnection(raw)
	if err == nil || !strings.Contains(err.Error(), `jump host "missing" not found`) {
		t.Fatalf("err = %v", err)
	}
}

func jumpTestConfig() Config {
	return Config{
		Defaults: Defaults{HostKeyCheck: HostKeyCheckInsecure},
		AuthProfiles: []AuthProfile{
			{Name: "ops", User: "root", Password: "secret"},
		},
		Hosts: []Host{
			{Name: "bastion", IP: "1.2.3.4", AuthRef: "ops"},
			{Name: "inner-db", IP: "10.0.0.8", AuthRef: "ops", Jump: "bastion"},
		},
	}
}
