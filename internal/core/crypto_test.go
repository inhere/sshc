package core

import (
	"os"
	"strings"
	"testing"
)

func TestEncryptPasswordRoundTrip(t *testing.T) {
	withTempConfig(t)
	encrypted, err := EncryptPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "secret" || !strings.HasPrefix(encrypted, "v1:") {
		t.Fatalf("encrypted password = %q", encrypted)
	}
	plain, err := DecryptPassword(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "secret" {
		t.Fatalf("plain = %q, want secret", plain)
	}
}

func TestDecryptPasswordMissingKeyDoesNotCreateNewKey(t *testing.T) {
	withTempConfig(t)
	_, err := DecryptPassword("v1:AAAA")
	if err == nil || !strings.Contains(err.Error(), "password key file not found") {
		t.Fatalf("err = %v", err)
	}
	keyPath, err := PasswordKeyPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("key file should not be created, err=%v", err)
	}
}
