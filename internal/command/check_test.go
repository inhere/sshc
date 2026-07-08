package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/inhere/sshc/internal/core"
)

func TestCheckCommandRequiresSource(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	err := app.RunWithArgs([]string{"check"})
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckCommandRejectsMultipleSources(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	err := app.RunWithArgs([]string{"check", "devhost", "--all"})
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckCommandFromTargetJSON(t *testing.T) {
	withTempConfig(t)
	if err := saveCheckCommandHosts(); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	if err := app.RunWithArgs([]string{"check", "lxc-app", "--json"}); err != nil {
		t.Fatalf("check: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, `"name": "lxc-app"`) || !strings.Contains(output, `"command_proxy": true`) {
		t.Fatalf("output = %q", output)
	}
}

func TestCheckCommandFromGroupAndTag(t *testing.T) {
	withTempConfig(t)
	if err := saveCheckCommandHosts(); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	if err := newTestApp().RunWithArgs([]string{"check", "--group", "lxc"}); err != nil {
		t.Fatalf("check group: %v", err)
	}
	if !strings.Contains(out.String(), "lxc-app") {
		t.Fatalf("group output = %q", out.String())
	}

	out.Reset()
	if err := newTestApp().RunWithArgs([]string{"check", "--tag", "container"}); err != nil {
		t.Fatalf("check tag: %v", err)
	}
	if !strings.Contains(out.String(), "lxc-app") {
		t.Fatalf("tag output = %q", out.String())
	}
}

func TestCheckCommandAll(t *testing.T) {
	withTempConfig(t)
	if err := saveCheckCommandHosts(); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	if err := app.RunWithArgs([]string{"check", "--all"}); err != nil {
		t.Fatalf("check all: %v", err)
	}
	if !strings.Contains(out.String(), "lxc-app") {
		t.Fatalf("all output = %q", out.String())
	}
}

func saveCheckCommandHosts() error {
	return core.SaveConfig(&core.Config{
		Hosts: []core.Host{{
			Name:        "lxc-app",
			Group:       "lxc",
			Tags:        []string{"container"},
			Backend:     core.HostBackendCommandProxy,
			Via:         "pve-host",
			RunTemplate: "pct exec 101 -- sh -lc {{cmd}}",
		}},
	})
}
