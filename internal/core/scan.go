package core

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// Scan discovery sources and families reported by `sshc scan`.
const (
	ScanSourceCIDR     = "cidr"
	ScanSourceNeighbor = "neighbor"

	ScanFamilyIPv4 = "ipv4"
	ScanFamilyIPv6 = "ipv6"

	// DefaultScanMaxHosts bounds one scan run so a wide --cidr cannot flood a
	// small switch.
	DefaultScanMaxHosts = 4096
)

// ScanTarget is one address considered by `sshc scan`. Address keeps an IPv6
// zone out of the host part; Zone carries it separately.
type ScanTarget struct {
	Address   string `json:"address"`
	Family    string `json:"family"`
	Zone      string `json:"zone,omitempty"`
	Interface string `json:"interface,omitempty"`
	Source    string `json:"source"`
}

// ScanSourceStatus reports whether a discovery source could be read.
type ScanSourceStatus struct {
	Source    string `json:"source"`
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
}

// ScanOptions controls target enumeration.
type ScanOptions struct {
	MaxHosts int
	Force    bool
}

type scanPrefix struct {
	prefix netip.Prefix
	iface  string
	own    netip.Addr
	source string
}

func (p scanPrefix) target(addr netip.Addr) ScanTarget {
	return ScanTarget{
		Address:   addr.String(),
		Family:    scanFamily(addr),
		Zone:      addr.Zone(),
		Interface: p.iface,
		Source:    p.source,
	}
}

// BuildScanTargets resolves the probe targets: explicit --cidr prefixes when
// given, otherwise the local IPv4 interface networks. Non private/link-local
// prefixes are refused unless Force is set.
func BuildScanTargets(cidrs []string, interfaces []string, opts ScanOptions) ([]ScanTarget, []ScanSourceStatus, error) {
	maxHosts := opts.MaxHosts
	if maxHosts <= 0 {
		maxHosts = DefaultScanMaxHosts
	}

	var prefixes []scanPrefix
	if len(cidrs) > 0 {
		for _, value := range cidrs {
			prefix, err := parseScanPrefix(value)
			if err != nil {
				return nil, nil, err
			}
			if !opts.Force && !isScannablePrefix(prefix) {
				return nil, nil, fmt.Errorf("scan cidr %s is not a private, link-local or loopback network; pass --force to scan it anyway", prefix)
			}
			prefixes = append(prefixes, scanPrefix{prefix: prefix, source: ScanSourceCIDR})
		}
	} else {
		local, err := localScanPrefixes(interfaces)
		if err != nil {
			return nil, nil, err
		}
		prefixes = local
	}

	targets, statuses, err := buildTargetsFromPrefixes(prefixes, maxHosts, len(cidrs) > 0)
	if err != nil {
		return nil, nil, err
	}
	return targets, statuses, nil
}

// buildTargetsFromPrefixes expands the prefixes under the host budget. Explicit
// --cidr input fails when over budget; auto-discovered interface networks are
// skipped so one huge virtual subnet cannot break the run.
func buildTargetsFromPrefixes(prefixes []scanPrefix, maxHosts int, explicit bool) ([]ScanTarget, []ScanSourceStatus, error) {
	targets := make([]ScanTarget, 0, maxHosts)
	seen := make(map[string]struct{}, maxHosts)
	var skipped []string
	for _, p := range prefixes {
		batch, err := enumScanPrefix(p, maxHosts-len(targets))
		if err != nil {
			if explicit {
				return nil, nil, err
			}
			// A directly connected switch is often next to a huge virtual subnet.
			skipped = append(skipped, p.prefix.String())
			continue
		}
		for _, target := range batch {
			key := target.Address + "%" + target.Zone
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, target)
		}
	}

	detail := fmt.Sprintf("%d target(s)", len(targets))
	if len(skipped) > 0 {
		detail += fmt.Sprintf("; skipped %d network(s) over --max-hosts %d: %s", len(skipped), maxHosts, strings.Join(skipped, ", "))
	}
	statuses := []ScanSourceStatus{{Source: ScanSourceCIDR, Available: true, Detail: detail}}
	return targets, statuses, nil
}

// ScanDialHost returns the host:port part handed to the dialer, keeping IPv6
// zones inside the bracketed host.
func ScanDialHost(target ScanTarget) string {
	host := target.Address
	if target.Zone != "" && !strings.Contains(host, "%") {
		host += "%" + target.Zone
	}
	return host
}

// IsScannableTarget reports whether a target belongs to a private, link-local
// or loopback network, i.e. the networks `sshc scan` is allowed to touch.
func IsScannableTarget(target ScanTarget) bool {
	addr, err := netip.ParseAddr(ScanDialHost(target))
	if err != nil {
		return false
	}
	return isScannableAddr(addr)
}

func isScannableAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	return addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLoopback()
}

func isScannablePrefix(prefix netip.Prefix) bool {
	return isScannableAddr(prefix.Masked().Addr()) || isScannableAddr(prefixLastAddr(prefix))
}

func parseScanPrefix(value string) (netip.Prefix, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return netip.Prefix{}, fmt.Errorf("scan cidr is empty")
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("invalid scan cidr %q: %w", value, err)
	}
	return prefix.Masked(), nil
}

// enumScanPrefix expands one prefix, skipping the IPv4 network/broadcast
// addresses and the local interface address when known.
func enumScanPrefix(p scanPrefix, budget int) ([]ScanTarget, error) {
	first := p.prefix.Masked().Addr()
	hostBits := first.BitLen() - p.prefix.Bits()
	if hostBits > 32 {
		return nil, fmt.Errorf("scan range %s is too large; narrow it with --cidr", p.prefix)
	}

	count := 1 << uint(hostBits)
	if count > budget {
		return nil, fmt.Errorf("scan range %s has %d hosts, more than the remaining --max-hosts budget %d; narrow it with --cidr", p.prefix, count, budget)
	}

	skipEdges := first.Is4() && p.prefix.Bits() <= 30
	last := prefixLastAddr(p.prefix)

	targets := make([]ScanTarget, 0, count)
	for addr := first; p.prefix.Contains(addr); addr = addr.Next() {
		if skipEdges && (addr == first || addr == last) {
			continue
		}
		if p.own.IsValid() && addr == p.own {
			continue
		}
		targets = append(targets, p.target(addr))
	}
	return targets, nil
}

func prefixLastAddr(prefix netip.Prefix) netip.Addr {
	bits := prefix.Bits()
	if prefix.Addr().Is4() {
		raw := prefix.Masked().Addr().As4()
		for i := 0; i < 4; i++ {
			for j := 0; j < 8; j++ {
				if i*8+j >= bits {
					raw[i] |= 1 << (7 - j)
				}
			}
		}
		return netip.AddrFrom4(raw)
	}

	raw := prefix.Masked().Addr().As16()
	for i := 0; i < 16; i++ {
		for j := 0; j < 8; j++ {
			if i*8+j >= bits {
				raw[i] |= 1 << (7 - j)
			}
		}
	}
	addr := netip.AddrFrom16(raw)
	return addr
}

func scanFamily(addr netip.Addr) string {
	if addr.Is4() || addr.Is4In6() {
		return ScanFamilyIPv4
	}
	return ScanFamilyIPv6
}

type ifaceAddrsFunc func(net.Interface) ([]net.Addr, error)

// localScanPrefixes derives the IPv4 networks of the local interfaces. The
// interface list keeps only the given names (or indexes as text) when non-empty.
func localScanPrefixes(names []string) ([]scanPrefix, error) {
	return localScanPrefixesFrom(net.Interfaces, func(iface net.Interface) ([]net.Addr, error) {
		return iface.Addrs()
	}, names)
}

func localScanPrefixesFrom(
	list func() ([]net.Interface, error),
	addrsOf ifaceAddrsFunc,
	names []string,
) ([]scanPrefix, error) {
	ifaces, err := list()
	if err != nil {
		return nil, fmt.Errorf("list local interfaces: %w", err)
	}

	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			wanted[name] = struct{}{}
		}
	}

	var prefixes []scanPrefix
	for _, iface := range ifaces {
		if len(wanted) > 0 {
			_, byName := wanted[iface.Name]
			_, byIndex := wanted[strconv.Itoa(iface.Index)]
			if !byName && !byIndex {
				continue
			}
		}
		addrs, err := addrsOf(iface)
		if err != nil {
			return nil, fmt.Errorf("read addresses of %s: %w", iface.Name, err)
		}
		for _, item := range addrs {
			ipNet, ok := item.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}
			ones, bits := ipNet.Mask.Size()
			if bits != 32 {
				continue
			}
			own, err := netip.ParseAddr(ip4.String())
			if err != nil {
				continue
			}
			prefix, err := own.Prefix(ones)
			if err != nil {
				continue
			}
			prefixes = append(prefixes, scanPrefix{
				prefix: prefix,
				iface:  iface.Name,
				own:    own,
				source: ScanSourceCIDR,
			})
		}
	}
	return prefixes, nil
}
