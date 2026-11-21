package core

import (
	"testing"

	"github.com/gookit/goutil/x/assert"
)

const windowsNeighborSample = `
Interface 12: ethernet_5

Internet Address                              Physical Address   Type
--------------------------------------------  -----------------  -----------
fe80::1c2b:3cff:fe4d:5e6f                     00-11-22-33-44-55  Reachable
fe80::abcd:1                                   00-11-22-33-44-66  Stale
ff02::1:2                                     33-33-00-01-00-02  Permanent
::1                                          00-11-22-33-44-77  Unreachable

Interface 1: loopback_0

Internet Address                              Physical Address   Type
--------------------------------------------  -----------------  -----------
`

const linuxNeighborSample = `
fe80::5054:ff:fe12:3456 dev eth0 lladdr 52:54:00:12:34:56 REACHABLE
fe80::1 dev eth0 lladdr 52:54:00:12:34:57 STALE
fe80::2 dev eth0  FAILED
fe80::3 dev eth0 lladdr 52:54:00:12:34:58 router STALE
fd00::9 dev enp3s0 lladdr 52:54:00:12:34:59 REACHABLE
`

func TestParseNeighborTable(t *testing.T) {
	t.Run("windows netsh table", func(t *testing.T) {
		is := assert.New(t)

		targets := parseNeighborTable(windowsNeighborSample)
		// ff02::1:2 is multicast, ::1 is loopback.
		is.Len(targets, 2)
		is.Eq("fe80::1c2b:3cff:fe4d:5e6f", targets[0].Address)
		is.Eq("12", targets[0].Zone)
		is.Eq("ethernet_5", targets[0].Interface)
		is.Eq(ScanFamilyIPv6, targets[0].Family)
		is.Eq(ScanSourceNeighbor, targets[0].Source)
		is.Eq("fe80::abcd:1", targets[1].Address)
	})

	t.Run("linux ip neigh table", func(t *testing.T) {
		is := assert.New(t)

		targets := parseNeighborTable(linuxNeighborSample)
		// The FAILED row is dropped.
		is.Len(targets, 4)
		is.Eq("fe80::5054:ff:fe12:3456", targets[0].Address)
		is.Eq("eth0", targets[0].Zone)
		is.Eq("eth0", targets[0].Interface)
		is.Eq("fe80::3", targets[2].Address)
		is.Eq("fd00::9", targets[3].Address)
		is.Eq("enp3s0", targets[3].Zone)
	})

	t.Run("empty table is not an error", func(t *testing.T) {
		is := assert.New(t)
		is.Len(parseNeighborTable(""), 0)
	})

	t.Run("link-local without zone is dropped", func(t *testing.T) {
		is := assert.New(t)
		is.Len(parseNeighborTable("fe80::99 lladdr 52:54:00:12:34:59 REACHABLE"), 0)
	})

	t.Run("global address without zone is kept", func(t *testing.T) {
		is := assert.New(t)

		targets := parseNeighborTable("2001:db8::5 lladdr 52:54:00:12:34:5a REACHABLE")
		is.Len(targets, 1)
		is.Eq("2001:db8::5", targets[0].Address)
	})
}

func TestReadIPv6Neighbors(t *testing.T) {
	is := assert.New(t)

	targets, status := ReadIPv6Neighbors()
	is.Eq(ScanSourceNeighbor, status.Source)
	if !status.Available {
		// A host without netsh/ip still gets a usable, explicit status.
		is.NotEmpty(status.Detail)
		is.Len(targets, 0)
		return
	}
	is.NotEmpty(status.Detail)
}
