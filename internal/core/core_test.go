package core

import (
	"path/filepath"
	"testing"
	"time"
)

func TestJoinRemotePath(t *testing.T) {
	tests := []struct {
		base string
		elem string
		want string
	}{
		{base: "/opt/app", elem: "a.txt", want: "/opt/app/a.txt"},
		{base: "/opt/app/", elem: "dir/a.txt", want: "/opt/app/dir/a.txt"},
		{base: ".", elem: "a.txt", want: "a.txt"},
	}
	for _, tt := range tests {
		if got := JoinRemotePath(tt.base, tt.elem); got != tt.want {
			t.Fatalf("JoinRemotePath(%q, %q) = %q, want %q", tt.base, tt.elem, got, tt.want)
		}
	}
}

func TestRemoteFilePath(t *testing.T) {
	tests := []struct {
		local  string
		remote string
		want   string
	}{
		{local: "local.txt", remote: "/tmp/remote.txt", want: "/tmp/remote.txt"},
		{local: "local.txt", remote: "/tmp/", want: "/tmp/local.txt"},
	}
	for _, tt := range tests {
		if got := RemoteFilePath(tt.local, tt.remote); got != tt.want {
			t.Fatalf("RemoteFilePath(%q, %q) = %q, want %q", tt.local, tt.remote, got, tt.want)
		}
	}
}

func TestParseTimeout(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
	}{
		{value: "", want: 0},
		{value: "5", want: 5 * time.Second},
		{value: "1500ms", want: 1500 * time.Millisecond},
		{value: "2m", want: 2 * time.Minute},
	}
	for _, tt := range tests {
		got, err := ParseTimeout(tt.value)
		if err != nil {
			t.Fatalf("ParseTimeout(%q): %v", tt.value, err)
		}
		if got != tt.want {
			t.Fatalf("ParseTimeout(%q) = %s, want %s", tt.value, got, tt.want)
		}
	}
}

func TestExpandUserPath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(SetUserHomeDirForTest(func() (string, error) { return home, nil }))

	got := expandUserPath("~/.ssh/id_rsa")
	want := filepath.Join(home, ".ssh", "id_rsa")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestLoginTermName(t *testing.T) {
	t.Setenv("TERM", "screen-256color")
	if got := loginTermName(""); got != "screen-256color" {
		t.Fatalf("term = %q, want screen-256color", got)
	}
	if got := loginTermName(" vt100 "); got != "vt100" {
		t.Fatalf("explicit term = %q, want vt100", got)
	}
	t.Setenv("TERM", "")
	if got := loginTermName(""); got != defaultPTYTerm {
		t.Fatalf("default term = %q, want %s", got, defaultPTYTerm)
	}
}

func withTempConfig(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	t.Cleanup(SetUserHomeDirForTest(func() (string, error) { return home, nil }))

	path := filepath.Join(home, ConfigFileName)
	t.Setenv(ConfigEnvKey, path)
	return path
}
