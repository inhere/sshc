package core

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const (
	CheckStatusOK      = "ok"
	CheckStatusFail    = "fail"
	CheckStatusSkipped = "-"
)

type CheckOptions struct {
	Timeout time.Duration
}

type CheckResult struct {
	Name         string   `json:"name"`
	Group        string   `json:"group"`
	Tags         []string `json:"tags,omitempty"`
	Address      string   `json:"address"`
	TCP          string   `json:"tcp"`
	SSH          string   `json:"ssh"`
	Auth         string   `json:"auth"`
	HostKey      string   `json:"host_key"`
	LatencyMS    int64    `json:"latency_ms,omitempty"`
	Error        string   `json:"error,omitempty"`
	Backend      string   `json:"backend,omitempty"`
	Via          string   `json:"via,omitempty"`
	CommandProxy bool     `json:"command_proxy,omitempty"`
}

var (
	checkTCPDial = func(network, address string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout(network, address, timeout)
	}
	checkSSHConnect = func(host Host) error {
		client, err := newSSHClientWithOptions(host, sshClientOptions{NoHostKeyPrompt: true})
		if err != nil {
			return err
		}
		return client.Close()
	}
)

func CheckHost(host Host, opts CheckOptions) CheckResult {
	started := time.Now()
	result := CheckResult{
		Name:    HostLogName(host),
		Group:   HostGroupName(host),
		Tags:    NormalizeTagList(host.Tags),
		Address: checkAddress(host),
		TCP:     CheckStatusSkipped,
		SSH:     CheckStatusSkipped,
		Auth:    CheckStatusSkipped,
		HostKey: CheckStatusSkipped,
		Backend: HostBackend(host),
	}
	if IsCommandProxyHost(host) {
		result.CommandProxy = true
		result.Via = strings.TrimSpace(host.Via)
		result.LatencyMS = SinceMS(started)
		if err := validateHost(host); err != nil {
			result.Error = err.Error()
			result.SSH = CheckStatusFail
			return result
		}
		result.TCP = CheckStatusSkipped
		result.SSH = CheckStatusOK
		result.Auth = CheckStatusSkipped
		result.HostKey = CheckStatusSkipped
		return result
	}
	if err := validateCheckHostConfig(host); err != nil {
		result.Error = err.Error()
		result.LatencyMS = SinceMS(started)
		return result
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = checkTimeout(host)
	}
	if err := checkTCP(host, timeout); err != nil {
		result.TCP = CheckStatusFail
		result.Error = err.Error()
		result.LatencyMS = SinceMS(started)
		return result
	}
	result.TCP = CheckStatusOK

	if err := checkKnownHostsPath(host); err != nil {
		result.HostKey = CheckStatusFail
		result.Error = err.Error()
		result.LatencyMS = SinceMS(started)
		return result
	}
	result.HostKey = CheckStatusOK

	if err := checkSSHConnect(host); err != nil {
		result.SSH = CheckStatusFail
		result.Auth = CheckStatusFail
		result.HostKey = checkHostKeyStatusFromError(err, result.HostKey)
		result.Error = err.Error()
		result.LatencyMS = SinceMS(started)
		return result
	}
	result.SSH = CheckStatusOK
	result.Auth = CheckStatusOK
	result.LatencyMS = SinceMS(started)
	return result
}

func checkTCP(host Host, timeout time.Duration) error {
	if strings.TrimSpace(host.Jump) != "" {
		return nil
	}
	conn, err := checkTCPDial("tcp", checkAddress(host), timeout)
	if err != nil {
		return err
	}
	return conn.Close()
}

func validateCheckHostConfig(host Host) error {
	if strings.TrimSpace(host.IP) == "" {
		return fmt.Errorf("ip is required")
	}
	if strings.TrimSpace(host.User) == "" {
		return fmt.Errorf("user is required for host %q", HostLogName(host))
	}
	if host.Password == "" && host.PasswordEnc == "" && strings.TrimSpace(host.KeyPath) == "" {
		return fmt.Errorf("password or key_path is required for host %q", HostLogName(host))
	}
	if strings.TrimSpace(host.KeyPath) != "" {
		path := expandUserPath(host.KeyPath)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("key_path %s: %w", path, err)
		}
	}
	if host.Port < 1 || host.Port > 65535 {
		return fmt.Errorf("invalid ssh port %d", host.Port)
	}
	return nil
}

func checkKnownHostsPath(host Host) error {
	if strings.TrimSpace(host.HostKeyCheck) == HostKeyCheckInsecure {
		return nil
	}
	path := knownHostsPath(host)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("known_hosts %s: %w", path, err)
	}
	return nil
}

func checkHostKeyStatusFromError(err error, current string) string {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "knownhosts") || strings.Contains(message, "known_hosts") || strings.Contains(message, "host key") {
		return CheckStatusFail
	}
	return current
}

func checkAddress(host Host) string {
	port := host.Port
	if port == 0 {
		port = DefaultSSHPort
	}
	return net.JoinHostPort(strings.TrimSpace(host.IP), fmt.Sprint(port))
}

func checkTimeout(host Host) time.Duration {
	timeout, err := clientConnectTimeout(host)
	if err != nil || timeout <= 0 {
		return 20 * time.Second
	}
	return timeout
}
