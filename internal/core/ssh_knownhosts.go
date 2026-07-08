package core

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

var defaultHostKeyAlgorithms = []string{
	ssh.CertAlgoRSASHA256v01,
	ssh.CertAlgoRSASHA512v01,
	ssh.CertAlgoRSAv01,
	ssh.InsecureCertAlgoDSAv01,
	ssh.CertAlgoECDSA256v01,
	ssh.CertAlgoECDSA384v01,
	ssh.CertAlgoECDSA521v01,
	ssh.CertAlgoED25519v01,
	ssh.CertAlgoSKECDSA256v01,
	ssh.CertAlgoSKED25519v01,
	ssh.KeyAlgoECDSA256,
	ssh.KeyAlgoECDSA384,
	ssh.KeyAlgoECDSA521,
	ssh.KeyAlgoRSASHA512,
	ssh.KeyAlgoRSASHA256,
	ssh.KeyAlgoRSA,
	ssh.InsecureKeyAlgoDSA,
	ssh.KeyAlgoED25519,
	ssh.KeyAlgoSKECDSA256,
	ssh.KeyAlgoSKED25519,
}

type HostKeyTrustResult struct {
	Host           Host
	Address        string
	KnownHostsPath string
	KeyType        string
	Fingerprint    string
	Status         string
}

type HostKeyTrustOptions struct {
	Force bool
}

func knownHostsPath(host Host) string {
	path := strings.TrimSpace(host.KnownHostsPath)
	if path == "" {
		path = DefaultKnownHostsPath
	}
	return expandUserPath(path)
}

func preferredHostKeyAlgorithms(host Host) ([]string, error) {
	switch strings.TrimSpace(host.HostKeyCheck) {
	case HostKeyCheckInsecure:
		return nil, nil
	case "", HostKeyCheckKnownHosts:
		types, err := knownHostKeyTypes(knownHostsPath(host), knownhosts.Normalize(knownHostAddress(host)))
		if err != nil {
			return nil, err
		}
		return mergePreferredHostKeyAlgorithms(types), nil
	default:
		return nil, fmt.Errorf("invalid host_key_check %q, want known_hosts or insecure", host.HostKeyCheck)
	}
}

func knownHostKeyTypes(path, normalizedHost string) ([]string, error) {
	if strings.TrimSpace(normalizedHost) == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read known_hosts %s: %w", path, err)
	}
	defer file.Close()

	seen := map[string]struct{}{}
	var types []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		hostPatterns, keyType, ok := knownHostLineHostAndKeyType(line)
		if !ok || !knownHostPatternsMatch(hostPatterns, normalizedHost) {
			continue
		}
		if _, ok := seen[keyType]; ok {
			continue
		}
		seen[keyType] = struct{}{}
		types = append(types, keyType)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read known_hosts %s: %w", path, err)
	}
	return types, nil
}

func knownHostLineHostAndKeyType(line string) (hostPatterns, keyType string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return "", "", false
	}
	hostFieldIndex := 0
	if strings.HasPrefix(fields[0], "@") {
		if fields[0] == "@revoked" {
			return "", "", false
		}
		hostFieldIndex = 1
	}
	if len(fields) <= hostFieldIndex+1 {
		return "", "", false
	}
	keyType = fields[hostFieldIndex+1]
	if fields[0] == "@cert-authority" {
		keyType = certHostKeyAlgorithm(keyType)
		if keyType == "" {
			return "", "", false
		}
	}
	return fields[hostFieldIndex], keyType, true
}

func certHostKeyAlgorithm(keyType string) string {
	switch keyType {
	case ssh.KeyAlgoRSA:
		return ssh.CertAlgoRSAv01
	case ssh.InsecureKeyAlgoDSA:
		return ssh.InsecureCertAlgoDSAv01
	case ssh.KeyAlgoECDSA256:
		return ssh.CertAlgoECDSA256v01
	case ssh.KeyAlgoECDSA384:
		return ssh.CertAlgoECDSA384v01
	case ssh.KeyAlgoECDSA521:
		return ssh.CertAlgoECDSA521v01
	case ssh.KeyAlgoSKECDSA256:
		return ssh.CertAlgoSKECDSA256v01
	case ssh.KeyAlgoED25519:
		return ssh.CertAlgoED25519v01
	case ssh.KeyAlgoSKED25519:
		return ssh.CertAlgoSKED25519v01
	default:
		return ""
	}
}

func knownHostPatternsMatch(patterns, normalizedHost string) bool {
	for _, pattern := range strings.Split(patterns, ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if pattern == normalizedHost || hashedKnownHostMatches(pattern, normalizedHost) {
			return true
		}
	}
	return false
}

func hashedKnownHostMatches(pattern, normalizedHost string) bool {
	parts := strings.Split(pattern, "|")
	if len(parts) != 4 || parts[0] != "" || parts[1] != "1" {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	mac := hmac.New(sha1.New, salt)
	_, _ = mac.Write([]byte(normalizedHost))
	return hmac.Equal(mac.Sum(nil), want)
}

func mergePreferredHostKeyAlgorithms(knownTypes []string) []string {
	if len(knownTypes) == 0 {
		return nil
	}
	known := map[string]struct{}{}
	for _, keyType := range knownTypes {
		keyType = strings.TrimSpace(keyType)
		if keyType != "" {
			known[keyType] = struct{}{}
		}
	}
	if len(known) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	algorithms := make([]string, 0, len(defaultHostKeyAlgorithms))
	add := func(algo string) {
		if _, ok := seen[algo]; ok {
			return
		}
		seen[algo] = struct{}{}
		algorithms = append(algorithms, algo)
	}
	for _, algo := range knownHostKeyAlgorithms(known) {
		add(algo)
	}
	for _, algo := range defaultHostKeyAlgorithms {
		add(algo)
	}
	return algorithms
}

func knownHostKeyAlgorithms(known map[string]struct{}) []string {
	var algorithms []string
	addKnown := func(keyType string, values ...string) {
		if _, ok := known[keyType]; ok {
			algorithms = append(algorithms, values...)
		}
	}

	addKnown(ssh.CertAlgoED25519v01, ssh.CertAlgoED25519v01)
	addKnown(ssh.CertAlgoSKED25519v01, ssh.CertAlgoSKED25519v01)
	addKnown(ssh.CertAlgoECDSA256v01, ssh.CertAlgoECDSA256v01)
	addKnown(ssh.CertAlgoECDSA384v01, ssh.CertAlgoECDSA384v01)
	addKnown(ssh.CertAlgoECDSA521v01, ssh.CertAlgoECDSA521v01)
	addKnown(ssh.CertAlgoSKECDSA256v01, ssh.CertAlgoSKECDSA256v01)
	addKnown(ssh.CertAlgoRSAv01, ssh.CertAlgoRSASHA256v01, ssh.CertAlgoRSASHA512v01, ssh.CertAlgoRSAv01)
	addKnown(ssh.InsecureCertAlgoDSAv01, ssh.InsecureCertAlgoDSAv01)
	addKnown(ssh.KeyAlgoED25519, ssh.KeyAlgoED25519)
	addKnown(ssh.KeyAlgoSKED25519, ssh.KeyAlgoSKED25519)
	addKnown(ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA256)
	addKnown(ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA384)
	addKnown(ssh.KeyAlgoECDSA521, ssh.KeyAlgoECDSA521)
	addKnown(ssh.KeyAlgoSKECDSA256, ssh.KeyAlgoSKECDSA256)
	addKnown(ssh.KeyAlgoRSA, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSA)
	addKnown(ssh.InsecureKeyAlgoDSA, ssh.InsecureKeyAlgoDSA)

	for keyType := range known {
		if !knownHostKeyTypeIsHandled(keyType) {
			algorithms = append(algorithms, keyType)
		}
	}
	return algorithms
}

func knownHostKeyTypeIsHandled(keyType string) bool {
	switch keyType {
	case ssh.CertAlgoRSAv01,
		ssh.InsecureCertAlgoDSAv01,
		ssh.CertAlgoECDSA256v01,
		ssh.CertAlgoECDSA384v01,
		ssh.CertAlgoECDSA521v01,
		ssh.CertAlgoED25519v01,
		ssh.CertAlgoSKECDSA256v01,
		ssh.CertAlgoSKED25519v01,
		ssh.KeyAlgoECDSA256,
		ssh.KeyAlgoECDSA384,
		ssh.KeyAlgoECDSA521,
		ssh.KeyAlgoRSA,
		ssh.InsecureKeyAlgoDSA,
		ssh.KeyAlgoED25519,
		ssh.KeyAlgoSKECDSA256,
		ssh.KeyAlgoSKED25519:
		return true
	default:
		return false
	}
}

func ensureKnownHostsFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create known_hosts directory %s: %w", filepath.Dir(path), err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0600)
	if err != nil {
		return fmt.Errorf("load known_hosts %s: %w", path, err)
	}
	return file.Close()
}

func trustOnUnknownHostKeyCallback(path string, callback ssh.HostKeyCallback) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := callback(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) || len(keyErr.Want) > 0 {
			return err
		}
		if err := trustUnknownHostKey(UnknownHostKey{
			Hostname:       hostname,
			Remote:         remote,
			Key:            key,
			KnownHostsPath: path,
		}); err != nil {
			return err
		}
		return nil
	}
}

type UnknownHostKey struct {
	Hostname       string
	Remote         net.Addr
	Key            ssh.PublicKey
	KnownHostsPath string
}

func trustUnknownHostKey(info UnknownHostKey) error {
	ok, err := confirmUnknownHostKey(info)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("host key for %s is unknown", info.Hostname)
	}
	return appendKnownHostKey(info.KnownHostsPath, info.Hostname, info.Key)
}

func promptConfirmUnknownHostKey(info UnknownHostKey) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, fmt.Errorf("host key for %s is unknown; run from an interactive terminal to trust it, pre-add it to %s, or set host_key_check=insecure", info.Hostname, info.KnownHostsPath)
	}
	fmt.Fprintf(os.Stderr, "The authenticity of host %q can't be established.\n", info.Hostname)
	fmt.Fprintf(os.Stderr, "%s key fingerprint is %s.\n", info.Key.Type(), ssh.FingerprintSHA256(info.Key))
	fmt.Fprintf(os.Stderr, "Add this host key to %s and continue? [y/N]: ", info.KnownHostsPath)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func appendKnownHostKey(path, hostname string, key ssh.PublicKey) error {
	if strings.TrimSpace(hostname) == "" {
		return errors.New("known_hosts hostname is required")
	}
	if err := ensureKnownHostsFile(path); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open known_hosts %s: %w", path, err)
	}
	defer file.Close()

	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	if _, err := file.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("write known_hosts %s: %w", path, err)
	}
	return nil
}

func TrustHostKey(host Host) (HostKeyTrustResult, error) {
	return TrustHostKeyWithOptions(host, HostKeyTrustOptions{})
}

func TrustHostKeyWithOptions(host Host, opts HostKeyTrustOptions) (HostKeyTrustResult, error) {
	if IsCommandProxyHost(host) {
		return HostKeyTrustResult{}, fmt.Errorf("host %s uses command_proxy backend; trust the via host instead", HostLogName(host))
	}
	if strings.TrimSpace(host.IP) == "" {
		return HostKeyTrustResult{}, errors.New("ip is required")
	}
	if host.Port == 0 {
		host.Port = DefaultSSHPort
	}

	path := knownHostsPath(host)
	if err := ensureKnownHostsFile(path); err != nil {
		return HostKeyTrustResult{}, err
	}
	key, remote, err := scanSSHHostKey(host)
	if err != nil {
		return HostKeyTrustResult{}, err
	}
	address := knownHostAddress(host)
	result := HostKeyTrustResult{
		Host:           host,
		Address:        address,
		KnownHostsPath: path,
		KeyType:        key.Type(),
		Fingerprint:    ssh.FingerprintSHA256(key),
	}

	callback, err := knownhosts.New(path)
	if err != nil {
		return HostKeyTrustResult{}, fmt.Errorf("load known_hosts %s: %w", path, err)
	}
	err = callback(address, remote, key)
	if err == nil {
		result.Status = "already_trusted"
		return result, nil
	}
	var keyErr *knownhosts.KeyError
	if !errors.As(err, &keyErr) {
		return HostKeyTrustResult{}, err
	}
	if len(keyErr.Want) > 0 {
		if !opts.Force {
			return HostKeyTrustResult{}, fmt.Errorf("host key for %s has changed; verify the host identity, then run host trust with --force to replace the stale known_hosts entry", address)
		}
		if _, err := removeKnownHostEntries(path, address); err != nil {
			return HostKeyTrustResult{}, err
		}
		if err := appendKnownHostKey(path, address, key); err != nil {
			return HostKeyTrustResult{}, err
		}
		result.Status = "replaced"
		return result, nil
	}
	if err := appendKnownHostKey(path, address, key); err != nil {
		return HostKeyTrustResult{}, err
	}
	result.Status = "added"
	return result, nil
}

func removeKnownHostEntries(path, address string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read known_hosts %s: %w", path, err)
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	trailingNewline := strings.HasSuffix(content, "\n")
	normalized := knownhosts.Normalize(address)
	kept := make([]string, 0, len(lines))
	removed := 0
	for i, line := range lines {
		if i == len(lines)-1 && line == "" && trailingNewline {
			continue
		}
		if knownHostLineMatches(line, normalized) {
			removed++
			continue
		}
		kept = append(kept, line)
	}
	next := strings.Join(kept, "\n")
	if trailingNewline && next != "" {
		next += "\n"
	}
	if err := os.WriteFile(path, []byte(next), 0600); err != nil {
		return removed, fmt.Errorf("write known_hosts %s: %w", path, err)
	}
	return removed, nil
}

func knownHostLineMatches(line, normalizedAddress string) bool {
	fields := strings.Fields(line)
	if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
		return false
	}
	hostFieldIndex := 0
	if strings.HasPrefix(fields[0], "@") {
		hostFieldIndex = 1
	}
	if len(fields) <= hostFieldIndex {
		return false
	}
	for _, pattern := range strings.Split(fields[hostFieldIndex], ",") {
		if pattern == normalizedAddress {
			return true
		}
	}
	return false
}

func knownHostAddress(host Host) string {
	port := host.Port
	if port == 0 {
		port = DefaultSSHPort
	}
	return net.JoinHostPort(strings.TrimSpace(host.IP), fmt.Sprint(port))
}
