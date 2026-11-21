package core

import (
	"net"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestBuildScanTargets(t *testing.T) {
	t.Run("cidr /30 keeps the two usable hosts", func(t *testing.T) {
		is := assert.New(t)

		targets, statuses, err := BuildScanTargets([]string{"192.168.1.0/30"}, nil, ScanOptions{})
		is.NoErr(err)
		is.Len(targets, 2)
		is.Eq("192.168.1.1", targets[0].Address)
		is.Eq("192.168.1.2", targets[1].Address)
		is.Eq(ScanFamilyIPv4, targets[0].Family)
		is.Eq(ScanSourceCIDR, targets[0].Source)
		is.Len(statuses, 1)
		is.True(statuses[0].Available)
	})

	t.Run("loopback /32 keeps the single host", func(t *testing.T) {
		is := assert.New(t)

		targets, _, err := BuildScanTargets([]string{"127.0.0.1/32"}, nil, ScanOptions{})
		is.NoErr(err)
		is.Len(targets, 1)
		is.Eq("127.0.0.1", targets[0].Address)
	})

	t.Run("wide cidr trips the host budget", func(t *testing.T) {
		is := assert.New(t)

		_, _, err := BuildScanTargets([]string{"10.0.0.0/8"}, nil, ScanOptions{MaxHosts: 256})
		is.Err(err)
		is.Contains(err.Error(), "--max-hosts")
	})

	t.Run("public cidr needs force", func(t *testing.T) {
		is := assert.New(t)

		_, _, err := BuildScanTargets([]string{"203.0.113.0/30"}, nil, ScanOptions{})
		is.Err(err)
		is.Contains(err.Error(), "--force")

		targets, _, err := BuildScanTargets([]string{"203.0.113.0/30"}, nil, ScanOptions{Force: true})
		is.NoErr(err)
		is.Len(targets, 2)
	})

	t.Run("link-local ipv6 cidr is accepted", func(t *testing.T) {
		is := assert.New(t)

		targets, _, err := BuildScanTargets([]string{"fe80::1/128"}, nil, ScanOptions{})
		is.NoErr(err)
		is.Len(targets, 1)
		is.Eq(ScanFamilyIPv6, targets[0].Family)
	})

	t.Run("invalid cidr is reported", func(t *testing.T) {
		is := assert.New(t)

		_, _, err := BuildScanTargets([]string{"192.168.1.0"}, nil, ScanOptions{})
		is.Err(err)
		is.Contains(err.Error(), "invalid scan cidr")
	})

	t.Run("local interfaces skip own address and edges", func(t *testing.T) {
		is := assert.New(t)

		ifaces := []net.Interface{{Index: 7, Name: "eth-test"}}
		prefixes, err := localScanPrefixesFrom(
			func() ([]net.Interface, error) { return ifaces, nil },
			func(net.Interface) ([]net.Addr, error) {
				_, ipNet, err := net.ParseCIDR("192.168.5.21/29")
				if err != nil {
					return nil, err
				}
				ipNet.IP = net.ParseIP("192.168.5.21").To4()
				return []net.Addr{ipNet}, nil
			},
			nil,
		)
		is.NoErr(err)
		is.Len(prefixes, 1)

		targets, err := enumScanPrefix(prefixes[0], DefaultScanMaxHosts)
		is.NoErr(err)
		// /29 = .16-.23: network .16 and broadcast .23 are skipped, the local
		// address .21 is skipped as well.
		is.Len(targets, 5)
		is.Eq("192.168.5.17", targets[0].Address)
		is.Eq("eth-test", targets[0].Interface)
		for _, target := range targets {
			is.NotEq("192.168.5.21", target.Address)
		}
	})

	t.Run("interface filter by name and index", func(t *testing.T) {
		is := assert.New(t)

		ifaces := []net.Interface{{Index: 1, Name: "lo-test"}, {Index: 9, Name: "eth-keep"}}
		addrsOf := func(iface net.Interface) ([]net.Addr, error) {
			_, ipNet, err := net.ParseCIDR("10.9.0.1/24")
			if err != nil {
				return nil, err
			}
			ipNet.IP = net.ParseIP("10.9.0.1").To4()
			return []net.Addr{ipNet}, nil
		}

		byName, err := localScanPrefixesFrom(func() ([]net.Interface, error) { return ifaces, nil }, addrsOf, []string{"eth-keep"})
		is.NoErr(err)
		is.Len(byName, 1)
		is.Eq("eth-keep", byName[0].iface)

		byIndex, err := localScanPrefixesFrom(func() ([]net.Interface, error) { return ifaces, nil }, addrsOf, []string{"9"})
		is.NoErr(err)
		is.Len(byIndex, 1)
		is.Eq("eth-keep", byIndex[0].iface)

		none, err := localScanPrefixesFrom(func() ([]net.Interface, error) { return ifaces, nil }, addrsOf, []string{"missing"})
		is.NoErr(err)
		is.Len(none, 0)
	})
}

func TestIsScannableTarget(t *testing.T) {
	tests := []struct {
		name   string
		target ScanTarget
		want   bool
	}{
		{name: "rfc1918", target: ScanTarget{Address: "10.1.2.3"}, want: true},
		{name: "link local v4", target: ScanTarget{Address: "169.254.10.20"}, want: true},
		{name: "loopback", target: ScanTarget{Address: "127.0.0.1"}, want: true},
		{name: "public v4", target: ScanTarget{Address: "203.0.113.5"}, want: false},
		{name: "link local v6 with zone", target: ScanTarget{Address: "fe80::1", Zone: "12"}, want: true},
		{name: "ula v6", target: ScanTarget{Address: "fd00::5"}, want: true},
		{name: "global v6", target: ScanTarget{Address: "2001:db8::5"}, want: false},
		{name: "invalid", target: ScanTarget{Address: "not-an-ip"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Eq(t, tt.want, IsScannableTarget(tt.target))
		})
	}
}

func TestScanDialHost(t *testing.T) {
	is := assert.New(t)

	is.Eq("fe80::1%12", ScanDialHost(ScanTarget{Address: "fe80::1", Zone: "12"}))
	is.Eq("192.168.1.5", ScanDialHost(ScanTarget{Address: "192.168.1.5"}))
	is.Eq("fe80::1%eth0", ScanDialHost(ScanTarget{Address: "fe80::1", Zone: "eth0"}))
}
