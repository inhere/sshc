package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultSecretCmdTimeout bounds one secret command run.
	DefaultSecretCmdTimeout = 10 * time.Second
	// SecretCmdTimeoutEnvKey overrides the secret command timeout.
	SecretCmdTimeoutEnvKey = "SSHC_SECRET_CMD_TIMEOUT"
)

// secretCmdRun is replaceable in tests so no real process is started.
var secretCmdRun = runSecretShellCommand

var secretCmdResults = struct {
	mu     sync.Mutex
	values map[string]string
}{values: map[string]string{}}

// ResolveSecretCommand returns the secret produced by command. Successful
// results are cached per command for the lifetime of the process, so a shared
// auth profile used by several hosts runs the command once; failures are not
// cached so a locked store can be retried.
func ResolveSecretCommand(command string, timeout time.Duration) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("secret command is empty")
	}
	if timeout <= 0 {
		timeout = secretCmdTimeout()
	}

	secretCmdResults.mu.Lock()
	cached, ok := secretCmdResults.values[command]
	secretCmdResults.mu.Unlock()
	if ok {
		return cached, nil
	}

	secret, err := secretCmdRun(command, timeout)
	if err != nil {
		return "", err
	}

	secretCmdResults.mu.Lock()
	secretCmdResults.values[command] = secret
	secretCmdResults.mu.Unlock()
	return secret, nil
}

// ResolveHostSecrets replaces command-based secret sources with their output.
// The resolved values live in memory only: the returned host has no commands
// left, so a second resolution inside the same call runs nothing.
func ResolveHostSecrets(host Host) (Host, error) {
	if command := strings.TrimSpace(host.PasswordCmd); command != "" {
		secret, err := ResolveSecretCommand(command, 0)
		if err != nil {
			return host, fmt.Errorf("host %q: password_cmd: %w", HostLogName(host), err)
		}
		host.Password = secret
		host.PasswordEnc = ""
		host.PasswordCmd = ""
	}
	if command := strings.TrimSpace(host.KeyPassphraseCmd); command != "" {
		secret, err := ResolveSecretCommand(command, 0)
		if err != nil {
			return host, fmt.Errorf("host %q: key_passphrase_cmd: %w", HostLogName(host), err)
		}
		host.KeyPassphrase = secret
		host.KeyPassphraseEnc = ""
		host.KeyPassphraseCmd = ""
	}
	return host, nil
}

// resetSecretCmdCacheForTest clears the process cache between test cases.
func resetSecretCmdCacheForTest() {
	secretCmdResults.mu.Lock()
	secretCmdResults.values = map[string]string{}
	secretCmdResults.mu.Unlock()
}

func secretCmdTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv(SecretCmdTimeoutEnvKey))
	if value == "" {
		return DefaultSecretCmdTimeout
	}
	timeout, err := ParseTimeout(value)
	if err != nil || timeout <= 0 {
		return DefaultSecretCmdTimeout
	}
	return timeout
}

// runSecretShellCommand executes command through the platform shell and keeps
// only its first stdout line.
func runSecretShellCommand(command string, timeout time.Duration) (string, error) {
	shell, args := secretShellCommand(command)
	if shell == "" {
		return "", errors.New("secret command shell not found; install pwsh, powershell or cmd")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, args...)
	stdout, err := cmd.Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("secret command timed out after %s", timeout)
	}
	if err != nil {
		message := fmt.Sprintf("secret command failed: %v", err)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if tail := strings.TrimSpace(string(exitErr.Stderr)); tail != "" {
				message += ": " + truncateSecretCmdText(tail)
			}
		}
		return "", errors.New(message)
	}

	secret, err := normalizeSecretCommandOutput(stdout)
	if err != nil {
		return "", err
	}
	return secret, nil
}

// normalizeSecretCommandOutput trims the trailing newlines and refuses empty or
// multi-line secrets.
func normalizeSecretCommandOutput(stdout []byte) (string, error) {
	secret := strings.TrimRight(string(stdout), "\r\n")
	if secret == "" {
		return "", errors.New("secret command produced no output")
	}
	if strings.ContainsAny(secret, "\r\n") {
		return "", errors.New("secret command produced multiple lines")
	}
	return secret, nil
}

func truncateSecretCmdText(value string) string {
	const limit = 200
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
