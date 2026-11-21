package command

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gookit/color"
	"github.com/gookit/gcli/v3"
	"github.com/gookit/goutil/x/assert"
	"github.com/inhere/sshc/internal/core"
)

func newScanTestApp() *gcli.App {
	app := gcli.NewApp()
	app.Name = "sshc"
	app.Add(NewScanCmd())
	return app
}

// stubScan replaces the three discovery seams and counts probe calls, so CLI
// cases never touch the network.
func stubScan(t *testing.T, results []core.ScanResult) *int {
	t.Helper()

	calls := 0
	oldBuild, oldNeighbors, oldProbe := scanBuildTargets, scanReadNeighbors, scanProbeSSH
	t.Cleanup(func() {
		scanBuildTargets, scanReadNeighbors, scanProbeSSH = oldBuild, oldNeighbors, oldProbe
	})

	scanBuildTargets = func(_ []string, _ []string, _ core.ScanOptions) ([]core.ScanTarget, []core.ScanSourceStatus, error) {
		return []core.ScanTarget{{Address: "192.168.1.20", Family: core.ScanFamilyIPv4, Source: core.ScanSourceCIDR}}, []core.ScanSourceStatus{
			{Source: core.ScanSourceCIDR, Available: true},
		}, nil
	}
	scanReadNeighbors = func() ([]core.ScanTarget, core.ScanSourceStatus) {
		return nil, core.ScanSourceStatus{Source: core.ScanSourceNeighbor, Available: false, Detail: "unsupported on test"}
	}
	scanProbeSSH = func(_ context.Context, _ []core.ScanTarget, _ int, _ time.Duration, _ int, _ core.ScanDialFunc) ([]core.ScanResult, error) {
		calls++
		return results, nil
	}
	return &calls
}

func TestScanCmd(t *testing.T) {
	t.Run("invalid flags exit 2 without probing", func(t *testing.T) {
		cases := [][]string{
			{"scan", "--parallel", "0"},
			{"scan", "--parallel", "2000"},
			{"scan", "--timeout", "20s"},
			{"scan", "--max-hosts", "0"},
			{"scan", "--port", "0"},
			{"scan", "--port", "70000"},
		}

		for _, args := range cases {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				is := assert.New(t)
				calls := stubScan(t, nil)
				defer setCommandOutputForTest(new(bytes.Buffer))()

				code := newScanTestApp().Run(args)
				is.Eq(gcli.ERR.ToInt(), code)
				is.Eq(0, *calls)
			})
		}
	})

	t.Run("extra positional argument exits 2", func(t *testing.T) {
		is := assert.New(t)
		calls := stubScan(t, nil)
		defer setCommandOutputForTest(new(bytes.Buffer))()

		code := newScanTestApp().Run([]string{"scan", "extra"})
		is.Eq(gcli.ERR.ToInt(), code)
		is.Eq(0, *calls)
	})

	t.Run("help lists the discovery flags", func(t *testing.T) {
		is := assert.New(t)
		buf := new(bytes.Buffer)
		defer setCommandOutputForTest(buf)()
		// gcli renders help through the color package output.
		color.Disable()
		color.SetOutput(buf)
		defer color.ResetOptions()

		code := newScanTestApp().Run([]string{"scan", "--help"})
		is.Eq(0, code)
		out := buf.String()
		for _, flag := range []string{"--cidr", "--interface", "--port", "--save", "--force", "--json", "--max-hosts"} {
			is.Contains(out, flag)
		}
	})

	t.Run("json output is parseable", func(t *testing.T) {
		is := assert.New(t)
		stubScan(t, []core.ScanResult{{
			Host:    "192.168.1.20",
			Address: "192.168.1.20:22",
			Family:  core.ScanFamilyIPv4,
			Port:    22,
			Banner:  "SSH-2.0-Test",
			Source:  core.ScanSourceCIDR,
		}})
		buf := new(bytes.Buffer)
		defer setCommandOutputForTest(buf)()

		code := newScanTestApp().Run([]string{"scan", "--json"})
		is.Eq(0, code)

		var results []core.ScanResult
		is.NoErr(json.Unmarshal(buf.Bytes(), &results))
		is.Len(results, 1)
		is.Eq("SSH-2.0-Test", results[0].Banner)
	})

	t.Run("table output keeps the fixed columns", func(t *testing.T) {
		is := assert.New(t)
		stubScan(t, []core.ScanResult{{
			Host:      "fe80::1",
			Address:   "[fe80::1%12]:22",
			Family:    core.ScanFamilyIPv6,
			Zone:      "12",
			Port:      22,
			Banner:    "SSH-2.0-OpenSSH_9.6",
			LatencyMS: 12,
			Source:    core.ScanSourceNeighbor,
		}})
		buf := new(bytes.Buffer)
		defer setCommandOutputForTest(buf)()

		code := newScanTestApp().Run([]string{"scan"})
		is.Eq(0, code)
		out := buf.String()
		for _, want := range []string{"Addr", "Zone", "Port", "Banner", "Latency", "Source", "SSH-2.0-OpenSSH_9.6"} {
			is.Contains(out, want)
		}
	})

	t.Run("alias discover dispatches the same command", func(t *testing.T) {
		is := assert.New(t)
		calls := stubScan(t, nil)
		defer setCommandOutputForTest(new(bytes.Buffer))()

		code := newScanTestApp().Run([]string{"discover", "--parallel", "1"})
		is.Eq(0, code)
		is.Eq(1, *calls)
	})
}

func TestScanSave(t *testing.T) {
	t.Run("saves discovered hosts without credentials", func(t *testing.T) {
		is := assert.New(t)
		withTempConfig(t)
		stubScan(t, []core.ScanResult{
			{Host: "192.168.1.20", Address: "192.168.1.20:22", Family: core.ScanFamilyIPv4, Port: 22, Banner: "SSH-2.0-Test"},
			{Host: "fe80::1c2b:3cff:fe4d:5e6f%12", Address: "[fe80::1c2b:3cff:fe4d:5e6f%12]:22", Family: core.ScanFamilyIPv6, Zone: "12", Port: 22},
		})
		defer setCommandOutputForTest(new(bytes.Buffer))()

		code := newScanTestApp().Run([]string{"scan", "--save", "--group", "sw1"})
		is.Eq(0, code)

		store := readTestStore(t)
		is.Len(store.Hosts, 2)
		is.Eq("lan-192-168-1-20", store.Hosts[0].Name)
		is.Eq("192.168.1.20", store.Hosts[0].IP)
		is.Eq(22, store.Hosts[0].Port)
		is.Eq("sw1", store.Hosts[0].Group)
		is.Eq("", store.Hosts[0].Password)
		is.Eq("", store.Hosts[0].KeyPath)
		is.Eq("", store.Hosts[0].AuthRef)
		is.Eq("fe80::1c2b:3cff:fe4d:5e6f%12", store.Hosts[1].IP)
		is.True(strings.HasPrefix(store.Hosts[1].Name, "lan-"))
	})

	t.Run("second run skips existing entries", func(t *testing.T) {
		is := assert.New(t)
		withTempConfig(t)
		stubScan(t, []core.ScanResult{
			{Host: "192.168.1.20", Address: "192.168.1.20:22", Family: core.ScanFamilyIPv4, Port: 22},
		})
		defer setCommandOutputForTest(new(bytes.Buffer))()

		is.Eq(0, newScanTestApp().Run([]string{"scan", "--save", "--group", "lan"}))
		code := newScanTestApp().Run([]string{"scan", "--save", "--group", "lan"})
		is.Eq(0, code)

		store := readTestStore(t)
		is.Len(store.Hosts, 1)
	})

	t.Run("name conflict gets a numeric suffix", func(t *testing.T) {
		is := assert.New(t)
		withTempConfig(t)
		if err := core.SaveConfig(&core.Config{Hosts: []core.Host{{Name: "lan-192-168-1-20", IP: "10.0.0.9", Port: 22}}}); err != nil {
			t.Fatal(err)
		}
		stubScan(t, []core.ScanResult{
			{Host: "192.168.1.20", Address: "192.168.1.20:22", Family: core.ScanFamilyIPv4, Port: 22},
		})
		defer setCommandOutputForTest(new(bytes.Buffer))()

		code := newScanTestApp().Run([]string{"scan", "--save", "--group", "lan"})
		is.Eq(0, code)

		store := readTestStore(t)
		is.Len(store.Hosts, 2)
		is.Eq("lan-192-168-1-20-2", store.Hosts[1].Name)
	})
}

func TestScanHostName(t *testing.T) {
	tests := []struct {
		name   string
		result core.ScanResult
		want   string
	}{
		{name: "ipv4", result: core.ScanResult{Host: "192.168.1.20"}, want: "lan-192-168-1-20"},
		{name: "ipv6 keeps last hextets", result: core.ScanResult{Host: "fe80::1c2b:3cff:fe4d:5e6f%12"}, want: "lan-1c2b-3cff-fe4d-5e6f"},
		{name: "short ipv6", result: core.ScanResult{Host: "fe80::1"}, want: "lan-fe80-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Eq(t, tt.want, ScanHostName(tt.result))
		})
	}
}
