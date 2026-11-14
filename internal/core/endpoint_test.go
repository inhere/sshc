package core

import "testing"

func TestParseHostTarget(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		host    string
		zone    string
		port    int
		hasPort bool
		wantErr bool
	}{
		{name: "ipv4", input: "192.0.2.10", host: "192.0.2.10"},
		{name: "hostname port", input: "example.test:2222", host: "example.test", port: 2222, hasPort: true},
		{name: "ipv6 zone", input: "fe80::1%12", host: "fe80::1%12", zone: "12"},
		{name: "bracketed ipv6 zone port", input: "[fe80::1%12]:2222", host: "fe80::1%12", zone: "12", port: 2222, hasPort: true},
		{name: "named zone", input: "fe80::1%Ethernet", host: "fe80::1%Ethernet", zone: "Ethernet"},
		{name: "unbracketed ipv6 port rejected", input: "fe80::1%12:2222", wantErr: true},
		{name: "bracketed ipv6 missing port", input: "[fe80::1%12]", wantErr: true},
		{name: "port zero rejected", input: "[fe80::1%12]:0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHostTarget(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Host != tt.host || got.Zone != tt.zone || got.Port != tt.port || got.HasPort != tt.hasPort {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestValidateRawTargetRejectsInlinePort(t *testing.T) {
	if err := validateRawTarget("fe80::1%12"); err != nil {
		t.Fatal(err)
	}
	if err := validateRawTarget("[fe80::1%12]:22"); err == nil {
		t.Fatal("expected inline port rejection")
	}
}

func TestNormalizeForwardRemoteAddrAcceptsScopedIPv6(t *testing.T) {
	got, err := normalizeForwardRemoteAddr("[fe80::1%12]:5432")
	if err != nil {
		t.Fatal(err)
	}
	if got != "[fe80::1%12]:5432" {
		t.Fatalf("got %q", got)
	}
	if _, err := normalizeForwardRemoteAddr("fe80::1%12:5432"); err == nil {
		t.Fatal("expected unbracketed scoped endpoint rejection")
	}
}
