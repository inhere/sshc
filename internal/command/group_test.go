package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/inhere/sshc/internal/core"
)

func TestGroupSetShowAndList(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}},
		Hosts:        []core.Host{{Name: "bastion", IP: "1.2.3.4", AuthRef: "dev-root", Port: 22}},
	}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"group", "set", "testing", "auth=dev-root", "jump=bastion", "port=2222", "connect_timeout=10s", "run_timeout=1m", "remote_script_dir=/var/tmp", "host_key_check=insecure", "known_hosts_path=~/.ssh/known_hosts"}); err != nil {
		t.Fatalf("group set: %v", err)
	}

	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	group := config.Groups["testing"]
	if group.AuthRef != "dev-root" || group.Jump != "bastion" || group.Port != 2222 || group.ConnectTimeout != "10s" || group.RunTimeout != "1m" {
		t.Fatalf("group = %+v", group)
	}

	var out bytes.Buffer
	t.Cleanup(setCommandOutputForTest(&out))
	if err := app.RunWithArgs([]string{"group", "show", "testing"}); err != nil {
		t.Fatalf("group show: %v", err)
	}
	if !strings.Contains(out.String(), `"auth_ref": "dev-root"`) || !strings.Contains(out.String(), `"port": 2222`) {
		t.Fatalf("show output = %q", out.String())
	}

	out.Reset()
	if err := app.RunWithArgs([]string{"group", "list"}); err != nil {
		t.Fatalf("group list: %v", err)
	}
	if !strings.Contains(out.String(), "testing") || !strings.Contains(out.String(), "dev-root") {
		t.Fatalf("list output = %q", out.String())
	}
}

func TestGroupUnsetMultipleFields(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{
		AuthProfiles: []core.AuthProfile{{Name: "dev-root", User: "root", KeyPath: "~/.ssh/id_rsa"}},
		Groups: map[string]core.GroupDefaults{
			"testing": {AuthRef: "dev-root", Port: 2222, Jump: "bastion", RunTimeout: "1m"},
		},
		Hosts: []core.Host{{Name: "bastion", IP: "1.2.3.4", AuthRef: "dev-root", Port: 22}},
	}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"group", "unset", "testing", "jump", "port", "run_timeout"}); err != nil {
		t.Fatalf("group unset: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	group := config.Groups["testing"]
	if group.Jump != "" || group.Port != 0 || group.RunTimeout != "" || group.AuthRef != "dev-root" {
		t.Fatalf("group = %+v", group)
	}
}

func TestGroupSetRejectsInvalidFieldWithoutSaving(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Groups: map[string]core.GroupDefaults{
		"testing": {User: "old"},
	}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	err := app.RunWithArgs([]string{"group", "set", "testing", "user=new", "bad=value"})
	if err == nil || !strings.Contains(err.Error(), "unknown group field") {
		t.Fatalf("err = %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Groups["testing"].User != "old" {
		t.Fatalf("group was changed: %+v", config.Groups["testing"])
	}
}

func TestGroupSetRejectsInvalidAuthRef(t *testing.T) {
	withTempConfig(t)
	app := newTestApp()
	err := app.RunWithArgs([]string{"group", "set", "testing", "auth=missing"})
	if err == nil || !strings.Contains(err.Error(), `missing auth profile "missing"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestGroupRemove(t *testing.T) {
	withTempConfig(t)
	if err := core.SaveConfig(&core.Config{Groups: map[string]core.GroupDefaults{"testing": {User: "root"}}}); err != nil {
		t.Fatal(err)
	}

	app := newTestApp()
	if err := app.RunWithArgs([]string{"group", "rm", "testing", "--yes"}); err != nil {
		t.Fatalf("group rm: %v", err)
	}
	config, err := core.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := config.Groups["testing"]; ok {
		t.Fatalf("groups = %+v", config.Groups)
	}
}
