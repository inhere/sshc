package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	passwordEncPrefix = "v1:"
	passwordKeyFile   = "key"
	passwordKeySize   = 32
)

func encryptStorePasswords(store Store) (Store, error) {
	config, err := encryptConfigPasswords(configFromStore(store))
	if err != nil {
		return Store{}, err
	}
	return storeFromConfig(config), nil
}

func encryptConfigPasswords(config Config) (Config, error) {
	if len(config.AuthProfiles) > 0 {
		profiles := make([]AuthProfile, len(config.AuthProfiles))
		copy(profiles, config.AuthProfiles)
		config.AuthProfiles = profiles
	}
	for i := range config.AuthProfiles {
		password := config.AuthProfiles[i].Password
		if password == "" {
			continue
		}
		encrypted, err := EncryptPassword(password)
		if err != nil {
			return Config{}, err
		}
		config.AuthProfiles[i].Password = ""
		config.AuthProfiles[i].PasswordEnc = encrypted
	}

	if len(config.Hosts) > 0 {
		hosts := make([]Host, len(config.Hosts))
		copy(hosts, config.Hosts)
		config.Hosts = hosts
	}
	for i := range config.Hosts {
		password := config.Hosts[i].Password
		if password == "" {
			continue
		}
		encrypted, err := EncryptPassword(password)
		if err != nil {
			return Config{}, err
		}
		config.Hosts[i].Password = ""
		config.Hosts[i].PasswordEnc = encrypted
	}
	return config, nil
}

func decryptStorePasswords(store *Store) error {
	config := configFromStore(*store)
	if err := decryptConfigPasswords(&config); err != nil {
		return err
	}
	*store = storeFromConfig(config)
	return nil
}

func decryptConfigPasswords(config *Config) error {
	for i := range config.AuthProfiles {
		if config.AuthProfiles[i].Password != "" || config.AuthProfiles[i].PasswordEnc == "" {
			continue
		}
		password, err := DecryptPassword(config.AuthProfiles[i].PasswordEnc)
		if err != nil {
			return err
		}
		config.AuthProfiles[i].Password = password
	}
	for i := range config.Hosts {
		if config.Hosts[i].Password != "" || config.Hosts[i].PasswordEnc == "" {
			continue
		}
		password, err := DecryptPassword(config.Hosts[i].PasswordEnc)
		if err != nil {
			return err
		}
		config.Hosts[i].Password = password
	}
	return nil
}

func EncryptPassword(password string) (string, error) {
	key, err := loadPasswordKey(true)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(password), nil)
	return passwordEncPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func DecryptPassword(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, passwordEncPrefix) {
		return "", errors.New("unsupported password encryption format")
	}
	key, err := loadPasswordKey(false)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	encoded := strings.TrimPrefix(value, passwordEncPrefix)
	sealed, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode encrypted password: %w", err)
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("encrypted password is too short")
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt password: %w", err)
	}
	return string(plain), nil
}

func loadPasswordKey(create bool) ([]byte, error) {
	path, err := PasswordKeyPath()
	if err != nil {
		return nil, err
	}
	key, err := os.ReadFile(path)
	if err == nil {
		key = []byte(strings.TrimSpace(string(key)))
		if len(key) == passwordKeySize {
			return key, nil
		}
		decoded, decodeErr := base64.RawStdEncoding.DecodeString(string(key))
		if decodeErr == nil && len(decoded) == passwordKeySize {
			return decoded, nil
		}
		return nil, fmt.Errorf("invalid password key file %s", path)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	if !create {
		return nil, fmt.Errorf("password key file not found: %s", path)
	}

	key = make([]byte, passwordKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0600); err != nil {
		return nil, err
	}
	return key, nil
}

func PasswordKeyPath() (string, error) {
	root, err := configRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, passwordKeyFile), nil
}
