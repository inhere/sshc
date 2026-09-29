package core

import (
	"strings"
	"testing"
)

func TestParseForwardRule(t *testing.T) {
	valid := []struct {
		input string
		local string
		want  string
	}{
		{"15432=127.0.0.1:5432", "127.0.0.1:15432", "127.0.0.1:5432"},
		{"15432=5432", "127.0.0.1:15432", "127.0.0.1:5432"},
		{"[::1]:15432=[::1]:5432", "[::1]:15432", "[::1]:5432"},
		{"127.0.0.1:15432=10.0.0.8:5432", "127.0.0.1:15432", "10.0.0.8:5432"},
		{"0=127.0.0.1:5432", "127.0.0.1:0", "127.0.0.1:5432"},
		{" localhost:15432 = db.internal:5432 ", "localhost:15432", "db.internal:5432"},
	}
	for _, tc := range valid {
		rule, err := ParseForwardRule(tc.input)
		if err != nil {
			t.Fatalf("ParseForwardRule(%q): %v", tc.input, err)
		}
		if rule.Local != tc.local || rule.Remote != tc.want {
			t.Fatalf("ParseForwardRule(%q) = %+v, want local=%s remote=%s", tc.input, rule, tc.local, tc.want)
		}
	}

	invalid := []struct {
		input string
		want  string
	}{
		{"", "required"},
		{"15432:127.0.0.1:5432", "want local=remote"},
		{"15432", "want local=remote"},
		{"15432=127.0.0.1:5432=extra", "want local=remote"},
		{"70000=127.0.0.1:5432", "invalid port"},
		{"127.0.0.1:15432=127.0.0.1:0", "invalid port"},
		{"8000-9000=127.0.0.1:5432", "port range"},
		{"/tmp/postgres.sock=127.0.0.1:5432", "unix socket"},
		{"udp://1.2.3.4:53=127.0.0.1:53", "only TCP"},
		{"15432=127.0.0.1", "want [host:]port"},
		{"15432=127.0.0.1:abc", "invalid port"},
	}
	for _, tc := range invalid {
		if _, err := ParseForwardRule(tc.input); err == nil {
			t.Fatalf("ParseForwardRule(%q) = nil error, want %q", tc.input, tc.want)
		} else if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("ParseForwardRule(%q) err = %v, want it to contain %q", tc.input, err, tc.want)
		}
	}
}

func tunnelTestConfig() Config {
	return Config{
		Version: ConfigVersion,
		AuthProfiles: []AuthProfile{
			{Name: "dev-root", User: "root", Password: "secret"},
		},
		Hosts: []Host{
			{Name: "devhost", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22},
			{Name: "bastion", IP: "10.0.0.9", User: "root", Password: "secret", Port: 22},
			{Name: "lxc-app", Backend: HostBackendCommandProxy, Via: "devhost", RunTemplate: "pct exec 100 -- {{cmd}}"},
		},
	}
}

func TestValidateTunnelProfile(t *testing.T) {
	cfg := tunnelTestConfig()
	base := TunnelProfile{
		Name:     "dev-db",
		Target:   "devhost",
		AuthRef:  "dev-root",
		Forwards: []TunnelForward{{Local: "15432", Remote: "127.0.0.1:5432"}},
	}
	if err := ValidateTunnelProfile(cfg, base); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}

	addressProfile := TunnelProfile{
		Name:     "prod-redis",
		Address:  "192.168.1.20",
		AuthRef:  "dev-root",
		Port:     2222,
		Forwards: []TunnelForward{{Local: "16379", Remote: "6379"}},
	}
	if err := ValidateTunnelProfile(cfg, addressProfile); err != nil {
		t.Fatalf("valid address profile rejected: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*TunnelProfile)
		wantErr string
	}{
		{"empty name", func(p *TunnelProfile) { p.Name = "" }, "name is required"},
		{"no target", func(p *TunnelProfile) { p.Target = "" }, "target or address is required"},
		{"both target and address", func(p *TunnelProfile) { p.Address = "192.168.1.20" }, "mutually exclusive"},
		{"address without auth", func(p *TunnelProfile) { p.Target, p.Address, p.AuthRef = "", "192.168.1.20", "" }, "--auth is required"},
		{"unknown auth", func(p *TunnelProfile) { p.AuthRef = "missing" }, `auth profile "missing" not found`},
		{"port out of range", func(p *TunnelProfile) { p.Port = 70000 }, "invalid tunnel port"},
		{"unknown jump", func(p *TunnelProfile) { p.Jump = "missing" }, `jump host "missing" not found`},
		{"no forwards", func(p *TunnelProfile) { p.Forwards = nil }, "at least one --forward"},
		{"non loopback local", func(p *TunnelProfile) {
			p.Forwards = []TunnelForward{{Local: "192.168.1.5:15432", Remote: "127.0.0.1:5432"}}
		}, "loopback"},
		{"wildcard local", func(p *TunnelProfile) {
			p.Forwards = []TunnelForward{{Local: "0.0.0.0:15432", Remote: "127.0.0.1:5432"}}
		}, "loopback"},
		{"duplicate local", func(p *TunnelProfile) {
			p.Forwards = []TunnelForward{
				{Local: "15432", Remote: "127.0.0.1:5432"},
				{Local: "127.0.0.1:15432", Remote: "127.0.0.1:6379"},
			}
		}, "duplicate local endpoint"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile := base
			profile.Forwards = append([]TunnelForward(nil), base.Forwards...)
			tc.mutate(&profile)
			err := ValidateTunnelProfile(cfg, profile)
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}

	zeroPorts := base
	zeroPorts.Forwards = []TunnelForward{
		{Local: "0", Remote: "127.0.0.1:5432"},
		{Local: "0", Remote: "127.0.0.1:6379"},
	}
	if err := ValidateTunnelProfile(cfg, zeroPorts); err != nil {
		t.Fatalf("two :0 rules must be allowed: %v", err)
	}
}

func TestCanonicalTunnelTarget(t *testing.T) {
	cfg := tunnelTestConfig()
	name, err := CanonicalTunnelTarget(cfg, "devho")
	if err != nil {
		t.Fatalf("CanonicalTunnelTarget: %v", err)
	}
	if name != "devhost" {
		t.Fatalf("canonical name = %q, want devhost", name)
	}
	if _, err := CanonicalTunnelTarget(cfg, "missing-host"); err == nil {
		t.Fatal("expected an error for an unknown host")
	}
}

func TestResolveTunnelHost(t *testing.T) {
	cfg := tunnelTestConfig()

	profile := TunnelProfile{
		Name:     "dev-db",
		Target:   "devhost",
		AuthRef:  "dev-root",
		Port:     2222,
		Jump:     "bastion",
		Forwards: []TunnelForward{{Local: "15432", Remote: "127.0.0.1:5432"}},
	}
	host, err := ResolveTunnelHost(cfg, profile)
	if err != nil {
		t.Fatalf("ResolveTunnelHost: %v", err)
	}
	if host.Name != "devhost" || host.IP != "10.0.0.8" || host.User != "root" {
		t.Fatalf("resolved host = %+v", host)
	}
	if host.Port != 2222 {
		t.Fatalf("port = %d, want the profile override 2222", host.Port)
	}
	if host.Jump != "bastion" {
		t.Fatalf("jump = %q, want bastion", host.Jump)
	}

	addressProfile := TunnelProfile{
		Name:     "prod-redis",
		Address:  "192.168.1.20",
		AuthRef:  "dev-root",
		Forwards: []TunnelForward{{Local: "16379", Remote: "127.0.0.1:6379"}},
	}
	host, err = ResolveTunnelHost(cfg, addressProfile)
	if err != nil {
		t.Fatalf("ResolveTunnelHost(address): %v", err)
	}
	if host.IP != "192.168.1.20" || host.User != "root" {
		t.Fatalf("address target host = %+v", host)
	}

	// A renamed or deleted host must fail loudly, never degrade to a raw target.
	renamed := Config{
		Version:      ConfigVersion,
		AuthProfiles: cfg.AuthProfiles,
		Hosts:        []Host{{Name: "devhost2", IP: "10.0.0.8", User: "root", Password: "secret", Port: 22}},
	}
	if _, err := ResolveTunnelHost(renamed, profile); err == nil {
		t.Fatal("expected host not found for a renamed target")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want a not found error", err)
	}

	noAuth := addressProfile
	noAuth.AuthRef = ""
	if _, err := ResolveTunnelHost(cfg, noAuth); err == nil || !strings.Contains(err.Error(), "--auth") {
		t.Fatalf("err = %v, want an --auth error", err)
	}

	proxyProfile := TunnelProfile{
		Name:     "proxy",
		Target:   "lxc-app",
		Forwards: []TunnelForward{{Local: "15432", Remote: "127.0.0.1:5432"}},
	}
	if _, err := ResolveTunnelHost(cfg, proxyProfile); err == nil || !strings.Contains(err.Error(), "command_proxy") {
		t.Fatalf("err = %v, want a command_proxy error", err)
	}
}

func TestTunnelStoreHelpers(t *testing.T) {
	profiles := []TunnelProfile{{
		Name:     "dev-db",
		Target:   "devhost",
		AuthRef:  "dev-root",
		Jump:     "bastion",
		Forwards: []TunnelForward{{Local: "15432", Remote: "127.0.0.1:5432"}},
	}}

	if _, ok := FindTunnel(profiles, "dev-db"); !ok {
		t.Fatal("FindTunnel did not find dev-db")
	}

	updated, err := UpsertTunnel(profiles, TunnelProfile{Name: "dev-db", Target: "devhost"}, false)
	if err == nil {
		t.Fatalf("UpsertTunnel without force = %v, want a duplicate error", updated)
	}
	replacement := TunnelProfile{Name: "dev-db", Target: "devhost", Forwards: []TunnelForward{{Local: "15433", Remote: "127.0.0.1:5432"}}}
	updated, err = UpsertTunnel(profiles, replacement, true)
	if err != nil {
		t.Fatalf("UpsertTunnel with force: %v", err)
	}
	if len(updated) != 1 || updated[0].Forwards[0].Local != "15433" {
		t.Fatalf("updated = %+v", updated)
	}
	added, err := UpsertTunnel(profiles, TunnelProfile{Name: "prod", Address: "192.168.1.20", AuthRef: "dev-root"}, false)
	if err != nil || len(added) != 2 {
		t.Fatalf("added = %+v err = %v", added, err)
	}
	if got := RemoveTunnel(added, "dev-db"); len(got) != 1 || got[0].Name != "prod" {
		t.Fatalf("removed = %+v", got)
	}

	if names := TunnelsUsingAuth(profiles, "dev-root"); len(names) != 1 || names[0] != "dev-db" {
		t.Fatalf("TunnelsUsingAuth = %v", names)
	}
	if names := TunnelsUsingHost(profiles, "bastion"); len(names) != 1 || names[0] != "dev-db" {
		t.Fatalf("TunnelsUsingHost(jump) = %v", names)
	}
	if names := TunnelsUsingHost(profiles, "devhost"); len(names) != 1 {
		t.Fatalf("TunnelsUsingHost(target) = %v", names)
	}
}

func TestTunnelProfileForwardRules(t *testing.T) {
	profile := TunnelProfile{
		Name: "dev-db",
		Forwards: []TunnelForward{
			{Local: "15432", Remote: "5432"},
			{Local: "[::1]:15433", Remote: "10.0.0.8:6379"},
		},
	}
	rules, err := profile.ForwardRules()
	if err != nil {
		t.Fatalf("ForwardRules: %v", err)
	}
	want := []ForwardRule{
		{LocalAddr: "127.0.0.1:15432", RemoteAddr: "127.0.0.1:5432"},
		{LocalAddr: "[::1]:15433", RemoteAddr: "10.0.0.8:6379"},
	}
	if len(rules) != len(want) {
		t.Fatalf("rules = %+v", rules)
	}
	for i := range want {
		if rules[i] != want[i] {
			t.Fatalf("rule[%d] = %+v, want %+v", i, rules[i], want[i])
		}
	}

	broken := TunnelProfile{Name: "broken", Forwards: []TunnelForward{{Local: "15432", Remote: "127.0.0.1"}}}
	if _, err := broken.ForwardRules(); err == nil {
		t.Fatal("expected an error for a malformed forward rule")
	}
}

func TestNormalizeTunnelProfile(t *testing.T) {
	profile := TunnelProfile{
		Name:     "  dev-db ",
		Target:   " devhost ",
		AuthRef:  " dev-root ",
		Forwards: []TunnelForward{{Local: " 15432 ", Remote: " 5432 "}},
	}
	NormalizeTunnelProfile(&profile)
	if profile.Name != "dev-db" || profile.Target != "devhost" || profile.AuthRef != "dev-root" {
		t.Fatalf("normalized profile = %+v", profile)
	}
	if profile.Forwards[0].Local != "127.0.0.1:15432" || profile.Forwards[0].Remote != "127.0.0.1:5432" {
		t.Fatalf("normalized forwards = %+v", profile.Forwards)
	}

	broken := TunnelProfile{Name: "broken", Forwards: []TunnelForward{{Local: "nope", Remote: "127.0.0.1:5432"}}}
	NormalizeTunnelProfile(&broken)
	if broken.Forwards[0].Local != "nope" {
		t.Fatalf("malformed rule must be kept for doctor reporting, got %+v", broken.Forwards)
	}

	empty := TunnelProfile{Name: "empty"}
	NormalizeTunnelProfile(&empty)
	if empty.Forwards == nil {
		t.Fatal("forwards must normalize to an empty slice, not nil")
	}
}
