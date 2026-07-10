package command

import (
	"path/filepath"
	"testing"
)

func TestNormalizeKeyPathForSaveKeepsHomeAndAbsolutePaths(t *testing.T) {
	homePath := "~/.ssh/id_rsa"
	if got, err := normalizeKeyPathForSave(" " + homePath + " "); err != nil || got != homePath {
		t.Fatalf("home path = %q, %v; want %q", got, err, homePath)
	}

	absPath := filepath.Join(t.TempDir(), "id_rsa")
	if got, err := normalizeKeyPathForSave(absPath); err != nil || got != absPath {
		t.Fatalf("absolute path = %q, %v; want %q", got, err, absPath)
	}
}
