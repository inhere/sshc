package core

import (
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestHostSecretCommandFields(t *testing.T) {
	t.Run("round trip keeps the command fields", func(t *testing.T) {
		is := assert.New(t)
		withTempConfig(t)

		config := &Config{Hosts: []Host{{
			Name:             "dev",
			IP:               "10.0.0.8",
			User:             "root",
			PasswordCmd:      "gopass show -on ssh/dev",
			KeyPassphraseCmd: "sops -d --extract pass secrets.yaml",
		}}}
		is.NoErr(SaveConfig(config))

		loaded, err := LoadConfig()
		is.NoErr(err)
		is.Len(loaded.Hosts, 1)
		is.Eq("gopass show -on ssh/dev", loaded.Hosts[0].PasswordCmd)
		is.Eq("sops -d --extract pass secrets.yaml", loaded.Hosts[0].KeyPassphraseCmd)
		is.Eq("", loaded.Hosts[0].Password)
	})

	t.Run("normalize trims the command fields", func(t *testing.T) {
		is := assert.New(t)

		host := Host{PasswordCmd: "  gopass show -on ssh/dev  ", KeyPassphraseCmd: "\tsops -d x\n"}
		NormalizeHostFields(&host)
		is.Eq("gopass show -on ssh/dev", host.PasswordCmd)
		is.Eq("sops -d x", host.KeyPassphraseCmd)
	})
}

func TestValidateHostSecretConflict(t *testing.T) {
	tests := []struct {
		name    string
		host    Host
		wantErr bool
	}{
		{
			name: "password only",
			host: Host{Name: "dev", IP: "10.0.0.8", User: "root", Port: 22, Password: "secret"},
		},
		{
			name: "password command only",
			host: Host{Name: "dev", IP: "10.0.0.8", User: "root", Port: 22, PasswordCmd: "gopass show ssh/dev"},
		},
		{
			name:    "password and command conflict",
			host:    Host{Name: "dev", IP: "10.0.0.8", User: "root", Port: 22, Password: "secret", PasswordCmd: "gopass show ssh/dev"},
			wantErr: true,
		},
		{
			name:    "passphrase and command conflict",
			host:    Host{Name: "dev", IP: "10.0.0.8", User: "root", Port: 22, KeyPath: "~/.ssh/id_ed25519", KeyPassphrase: "secret", KeyPassphraseCmd: "sops -d x"},
			wantErr: true,
		},
		{
			name: "passphrase command with key",
			host: Host{Name: "dev", IP: "10.0.0.8", User: "root", Port: 22, KeyPath: "~/.ssh/id_ed25519", KeyPassphraseCmd: "sops -d x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHost(tt.host)
			if tt.wantErr {
				assert.Err(t, err)
				assert.StrContains(t, err.Error(), "cannot both be set")
				return
			}
			assert.NoErr(t, err)
		})
	}
}

func TestCheckConfigSecretSourceConflicts(t *testing.T) {
	is := assert.New(t)

	config := Config{
		Hosts:        []Host{{Name: "dev", IP: "10.0.0.8", User: "root", Password: "x", PasswordCmd: "y"}},
		AuthProfiles: []AuthProfile{{Name: "ops", PasswordEnc: "enc", PasswordCmd: "gopass show ops"}},
	}
	issues := CheckConfig(config)
	is.True(HasDoctorErrors(issues))

	var messages []string
	for _, issue := range issues {
		if issue.Level == DoctorError {
			messages = append(messages, issue.Message)
		}
	}
	is.Len(messages, 2)
	is.Contains(messages[0], "password and password_cmd cannot both be set")
	is.Contains(messages[1], "password and password_cmd cannot both be set")
}

func TestApplyAuthProfileSecretCommands(t *testing.T) {
	t.Run("profile command clears the host value", func(t *testing.T) {
		is := assert.New(t)

		host := EffectiveHost{Host: Host{Password: "host-secret", PasswordEnc: "enc"}}
		applyAuthProfile(&host, AuthProfile{PasswordCmd: "gopass show ops"})
		is.Eq("gopass show ops", host.PasswordCmd)
		is.Eq("", host.Password)
		is.Eq("", host.PasswordEnc)
	})

	t.Run("profile value clears the host command", func(t *testing.T) {
		is := assert.New(t)

		host := EffectiveHost{Host: Host{PasswordCmd: "gopass show host"}}
		applyAuthProfile(&host, AuthProfile{Password: "profile-secret"})
		is.Eq("profile-secret", host.Password)
		is.Eq("", host.PasswordCmd)
	})

	t.Run("profile passphrase command clears the host passphrase", func(t *testing.T) {
		is := assert.New(t)

		host := EffectiveHost{Host: Host{KeyPath: "~/.ssh/id", KeyPassphrase: "phrase"}}
		applyAuthProfile(&host, AuthProfile{KeyPassphraseCmd: "sops -d phrase"})
		is.Eq("sops -d phrase", host.KeyPassphraseCmd)
		is.Eq("", host.KeyPassphrase)
	})
}
