package command

import (
	"testing"

	"github.com/inhere/sshc/internal/core"
)

func TestResolveCommandRouteUsesExistingLocalPort(t *testing.T) {
	host := core.Host{IP: "192.0.2.10", Port: 22}
	routed, route, err := resolveCommandRoute(host, 10022, "", "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if route != nil {
		t.Fatal("existing local port should not create a session")
	}
	if routed.DialAddress != "127.0.0.1:10022" {
		t.Fatalf("dial address = %q", routed.DialAddress)
	}
	if routed.IP != host.IP || routed.Port != host.Port {
		t.Fatalf("target identity changed: %+v", routed)
	}
}

func TestResolveCommandRouteRejectsSavedJump(t *testing.T) {
	_, _, err := resolveCommandRoute(core.Host{IP: "192.0.2.10", Port: 22, Jump: "bastion"}, 10022, "", "", false, false)
	if err == nil {
		t.Fatal("expected saved jump conflict")
	}
}
