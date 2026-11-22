package core

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// neighborCmdTimeout bounds the platform neighbor-table command.
const neighborCmdTimeout = 3 * time.Second

// ReadIPv6Neighbors reads the system IPv6 neighbor cache. A missing or
// unparsable source is reported through the status instead of failing the whole
// scan, so IPv4 discovery keeps working.
func ReadIPv6Neighbors() ([]ScanTarget, ScanSourceStatus) {
	status := ScanSourceStatus{Source: ScanSourceNeighbor}
	text, err := neighborTableText()
	if err != nil {
		status.Detail = err.Error()
		return nil, status
	}

	targets := parseNeighborTable(text)
	status.Available = true
	status.Detail = fmt.Sprintf("%d target(s)", len(targets))
	return targets, status
}

// parseNeighborTable handles both tables `sshc scan` reads:
//
//	Windows netsh:  "Interface 12: ethernet_5" header + "fe80::1  aa-bb-..  Reachable"
//	Linux ip -6:    "fe80::1 dev eth0 lladdr aa:bb:.. REACHABLE"
//
// The zone is the dialable scope of this host: the interface index on Windows,
// the interface name on Linux. Entries that cannot be dialed are dropped.
func parseNeighborTable(text string) []ScanTarget {
	var (
		targets          []ScanTarget
		currentIndex     string
		currentInterface string
	)

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if index, name, ok := parseNeighborInterfaceHeader(line); ok {
			currentIndex, currentInterface = index, name
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "-") {
			continue
		}

		addr, err := netip.ParseAddr(fields[0])
		if err != nil || !addr.Is6() || addr.Is4In6() {
			continue
		}
		if addr.IsLoopback() || addr.IsMulticast() || addr.IsUnspecified() {
			continue
		}

		zone, iface := currentIndex, currentInterface
		if dev, name := neighborDev(fields); dev != "" {
			zone, iface = dev, name
		}
		if neighborStateFailed(fields) {
			continue
		}
		if addr.IsLinkLocalUnicast() && zone == "" {
			// A link-local address without a scope cannot be dialed.
			continue
		}

		targets = append(targets, ScanTarget{
			Address:   addr.String(),
			Family:    ScanFamilyIPv6,
			Zone:      zone,
			Interface: iface,
			Source:    ScanSourceNeighbor,
		})
	}
	return targets
}

// parseNeighborInterfaceHeader matches `netsh` section headers such as
// "Interface 12: ethernet_5".
func parseNeighborInterfaceHeader(line string) (index, name string, ok bool) {
	rest, found := strings.CutPrefix(line, "Interface ")
	if !found {
		return "", "", false
	}
	index, name, found = strings.Cut(rest, ":")
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(index), strings.TrimSpace(name), true
}

// neighborDev returns the interface of an `ip -6 neigh` row ("dev <name>").
func neighborDev(fields []string) (dev, name string) {
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "dev" {
			return fields[i+1], fields[i+1]
		}
	}
	return "", ""
}

// neighborStateFailed drops rows the kernel could not resolve.
func neighborStateFailed(fields []string) bool {
	for _, field := range fields[1:] {
		switch strings.ToLower(field) {
		case "incomplete", "failed", "unreachable":
			return true
		}
	}
	return false
}

// neighborTableText runs the platform neighbor command.
func neighborTableText() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), neighborCmdTimeout)
	defer cancel()
	return runNeighborCommand(ctx)
}
