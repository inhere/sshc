package core

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type RunOptions struct {
	Timeout          time.Duration
	KillAfter        time.Duration
	Env              map[string]string
	CWD              string
	Sudo             bool
	SudoUser         string
	ScriptPath       string
	RemoteScriptDir  string
	RemoteScriptPath string
	KeepRemoteScript bool
}

func remoteTimeoutCommand(command string, opts RunOptions) string {
	if opts.Timeout <= 0 {
		return command
	}
	killAfter := effectiveKillAfter(opts.KillAfter)
	return "command -v timeout >/dev/null 2>&1 || { echo 'sshc: remote timeout command not found' >&2; exit 127; }; " +
		"timeout --kill-after=" + remoteDuration(killAfter) + " " + remoteDuration(opts.Timeout) + " bash -lc " + shellQuote(command)
}

func remoteSudoCommand(command string, opts RunOptions) string {
	if opts.SudoUser != "" {
		return "sudo -u " + shellQuote(opts.SudoUser) + " bash -lc " + shellQuote(command)
	}
	if opts.Sudo {
		return "sudo bash -lc " + shellQuote(command)
	}
	return command
}

func remoteClientTimeout(opts RunOptions) time.Duration {
	if opts.Timeout <= 0 {
		return 0
	}
	return opts.Timeout + effectiveKillAfter(opts.KillAfter) + clientTimeoutBuffer
}

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var sudoUserPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*[$]?$`)

func ParseTimeout(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, errors.New("timeout must be >= 0")
		}
		return time.Duration(seconds) * time.Second, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q: %w", value, err)
	}
	if d < 0 {
		return 0, errors.New("timeout must be >= 0")
	}
	return d, nil
}

func LoadRunEnv(file string, inline []string) (map[string]string, error) {
	env := make(map[string]string)
	if strings.TrimSpace(file) != "" {
		if err := loadEnvFile(env, file); err != nil {
			return nil, err
		}
	}
	for _, item := range inline {
		key, val, err := parseEnvAssignment(item)
		if err != nil {
			return nil, err
		}
		env[key] = val
	}
	return env, nil
}

func ValidateSudoUser(user string) error {
	user = strings.TrimSpace(user)
	if user == "" {
		return errors.New("sudo-user is required")
	}
	if !sudoUserPattern.MatchString(user) {
		return fmt.Errorf("invalid sudo-user %q", user)
	}
	return nil
}

func loadEnvFile(dst map[string]string, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("env-file path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, err := parseEnvAssignment(line)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		dst[key] = val
	}
	return scanner.Err()
}

func parseEnvAssignment(item string) (string, string, error) {
	key, val, ok := strings.Cut(item, "=")
	if !ok {
		return "", "", fmt.Errorf("env %q must use k=v format", item)
	}
	key = strings.TrimSpace(key)
	if !envNamePattern.MatchString(key) {
		return "", "", fmt.Errorf("invalid env name %q", key)
	}
	return key, trimEnvValue(val), nil
}

func trimEnvValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func BuildRemoteCommand(command string, env map[string]string) (string, error) {
	return BuildRemoteCommandWithCWD(command, env, "")
}

func BuildRemoteCommandWithCWD(command string, env map[string]string, cwd string) (string, error) {
	if len(env) == 0 {
		return withRemoteCWD(command, cwd), nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		if !envNamePattern.MatchString(key) {
			return "", fmt.Errorf("invalid env name %q", key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		parts = append(parts, key+"="+shellQuote(env[key]))
	}
	parts = append(parts, command)
	return withRemoteCWD(strings.Join(parts, " "), cwd), nil
}
