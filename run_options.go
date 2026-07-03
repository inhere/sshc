package main

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

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func parseTimeout(value string) (time.Duration, error) {
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

func normalizeEnvFile(envFile, efile string) (string, error) {
	envFile = strings.TrimSpace(envFile)
	efile = strings.TrimSpace(efile)
	if envFile != "" && efile != "" && envFile != efile {
		return "", errors.New("--env-file and --efile cannot be different")
	}
	if envFile != "" {
		return envFile, nil
	}
	return efile, nil
}

func loadRunEnv(file string, inline []string) (map[string]string, error) {
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

func buildRemoteCommand(command string, env map[string]string) (string, error) {
	if len(env) == 0 {
		return command, nil
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
	return strings.Join(parts, " "), nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
