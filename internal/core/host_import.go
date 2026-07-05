package core

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type HostImportFormat string

const (
	HostImportIPs   HostImportFormat = "ips"
	HostImportPlain HostImportFormat = "plain"
	HostImportCSV   HostImportFormat = "csv"
)

type HostImportDefaults struct {
	AuthRef         string
	User            string
	KeyPath         string
	Group           string
	Remark          string
	Port            int
	Jump            string
	ConnectTimeout  string
	RunTimeout      string
	RemoteScriptDir string
	HostKeyCheck    string
	KnownHostsPath  string
}

type HostImportError struct {
	Line    int
	Field   string
	Message string
}

func (e HostImportError) Error() string {
	var parts []string
	if e.Line > 0 {
		parts = append(parts, fmt.Sprintf("line %d", e.Line))
	}
	if strings.TrimSpace(e.Field) != "" {
		parts = append(parts, fmt.Sprintf("field %q", e.Field))
	}
	if len(parts) == 0 {
		return e.Message
	}
	return strings.Join(parts, ": ") + ": " + e.Message
}

func ParseHostImport(reader io.Reader, format HostImportFormat, defaults HostImportDefaults) ([]Host, []HostImportError) {
	switch format {
	case HostImportIPs:
		return ParseHostImportIPs(reader, defaults)
	case HostImportPlain:
		return ParseHostImportPlain(reader, defaults)
	case HostImportCSV:
		return ParseHostImportCSV(reader, defaults)
	default:
		return nil, []HostImportError{{Message: fmt.Sprintf("unsupported host import format %q", format)}}
	}
}

func ParseHostImportIPs(reader io.Reader, defaults HostImportDefaults) ([]Host, []HostImportError) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, []HostImportError{{Message: err.Error()}}
	}
	var hosts []Host
	var errs []HostImportError
	for idx, raw := range strings.Split(string(data), "\n") {
		lineNo := idx + 1
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		host := Host{IP: line}
		applyHostImportDefaults(&host, defaults)
		normalizeImportedHost(&host)
		if rowErrs := validateImportedHost(host, lineNo); len(rowErrs) > 0 {
			errs = append(errs, rowErrs...)
			continue
		}
		hosts = append(hosts, host)
	}
	return hosts, errs
}

func ParseHostImportPlain(reader io.Reader, defaults HostImportDefaults) ([]Host, []HostImportError) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, []HostImportError{{Message: err.Error()}}
	}
	var hosts []Host
	var errs []HostImportError
	var block []plainLine
	flush := func() {
		if len(block) == 0 {
			return
		}
		host, rowErrs := parseHostImportPlainBlock(block, defaults)
		if len(rowErrs) > 0 {
			errs = append(errs, rowErrs...)
		} else {
			hosts = append(hosts, host)
		}
		block = nil
	}

	for idx, raw := range strings.Split(string(data), "\n") {
		lineNo := idx + 1
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		block = append(block, plainLine{Line: lineNo, Text: line})
	}
	flush()
	return hosts, errs
}

func ParseHostImportCSV(reader io.Reader, defaults HostImportDefaults) ([]Host, []HostImportError) {
	r := csv.NewReader(reader)
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, []HostImportError{{Line: 1, Message: err.Error()}}
	}
	fields := make([]string, len(header))
	var errs []HostImportError
	for i, field := range header {
		key := normalizeHostImportField(field)
		if key == "" {
			errs = append(errs, HostImportError{Line: 1, Field: field, Message: "empty csv header field"})
			continue
		}
		if !isHostImportField(key) {
			errs = append(errs, HostImportError{Line: 1, Field: field, Message: "unknown csv header field"})
			continue
		}
		fields[i] = key
	}
	if len(errs) > 0 {
		return nil, errs
	}

	var hosts []Host
	lineNo := 1
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		lineNo++
		if err != nil {
			errs = append(errs, HostImportError{Line: lineNo, Message: err.Error()})
			continue
		}
		if csvRecordIsBlank(record) {
			continue
		}
		host := Host{}
		var rowErrs []HostImportError
		for i, value := range record {
			if i >= len(fields) {
				continue
			}
			if err := setHostImportField(&host, fields[i], strings.TrimSpace(value), lineNo); err != nil {
				rowErrs = append(rowErrs, *err)
			}
		}
		applyHostImportDefaults(&host, defaults)
		normalizeImportedHost(&host)
		rowErrs = append(rowErrs, validateImportedHost(host, lineNo)...)
		if len(rowErrs) > 0 {
			errs = append(errs, rowErrs...)
			continue
		}
		hosts = append(hosts, host)
	}
	return hosts, errs
}

type plainLine struct {
	Line int
	Text string
}

func parseHostImportPlainBlock(lines []plainLine, defaults HostImportDefaults) (Host, []HostImportError) {
	host := Host{}
	var errs []HostImportError
	for _, line := range lines {
		key, value, ok := cutHostImportKV(line.Text)
		if !ok {
			errs = append(errs, HostImportError{Line: line.Line, Message: "expected key=value or key: value"})
			continue
		}
		key = normalizeHostImportField(key)
		if !isHostImportField(key) {
			errs = append(errs, HostImportError{Line: line.Line, Field: key, Message: "unknown field"})
			continue
		}
		if err := setHostImportField(&host, key, value, line.Line); err != nil {
			errs = append(errs, *err)
		}
	}
	applyHostImportDefaults(&host, defaults)
	normalizeImportedHost(&host)
	if len(lines) > 0 {
		errs = append(errs, validateImportedHost(host, lines[0].Line)...)
	}
	return host, errs
}

func cutHostImportKV(line string) (string, string, bool) {
	if key, value, ok := strings.Cut(line, "="); ok {
		return strings.TrimSpace(key), strings.TrimSpace(value), true
	}
	if key, value, ok := strings.Cut(line, ":"); ok {
		return strings.TrimSpace(key), strings.TrimSpace(value), true
	}
	return "", "", false
}

func setHostImportField(host *Host, key, value string, line int) *HostImportError {
	if value == "" {
		return nil
	}
	switch key {
	case "ip":
		host.IP = value
	case "name":
		host.Name = value
	case "auth_ref":
		host.AuthRef = value
	case "user":
		host.User = value
	case "password":
		host.Password = value
	case "key_path":
		host.KeyPath = value
	case "group":
		host.Group = value
	case "remark":
		host.Remark = value
	case "port":
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return &HostImportError{Line: line, Field: key, Message: fmt.Sprintf("invalid ssh port %q", value)}
		}
		host.Port = port
	case "jump":
		host.Jump = value
	case "connect_timeout":
		host.ConnectTimeout = value
	case "run_timeout":
		host.RunTimeout = value
	case "remote_script_dir":
		host.RemoteScriptDir = value
	case "host_key_check":
		if value != HostKeyCheckKnownHosts && value != HostKeyCheckInsecure {
			return &HostImportError{Line: line, Field: key, Message: fmt.Sprintf("invalid host_key_check %q, want known_hosts or insecure", value)}
		}
		host.HostKeyCheck = value
	case "known_hosts_path":
		host.KnownHostsPath = value
	default:
		return &HostImportError{Line: line, Field: key, Message: "unknown field"}
	}
	return nil
}

func applyHostImportDefaults(host *Host, defaults HostImportDefaults) {
	setStringDefault(&host.AuthRef, defaults.AuthRef)
	setStringDefault(&host.User, defaults.User)
	setStringDefault(&host.KeyPath, defaults.KeyPath)
	setStringDefault(&host.Group, defaults.Group)
	setStringDefault(&host.Remark, defaults.Remark)
	setStringDefault(&host.Jump, defaults.Jump)
	setStringDefault(&host.ConnectTimeout, defaults.ConnectTimeout)
	setStringDefault(&host.RunTimeout, defaults.RunTimeout)
	setStringDefault(&host.RemoteScriptDir, defaults.RemoteScriptDir)
	setStringDefault(&host.HostKeyCheck, defaults.HostKeyCheck)
	setStringDefault(&host.KnownHostsPath, defaults.KnownHostsPath)
	if host.Port == 0 && defaults.Port > 0 {
		host.Port = defaults.Port
	}
}

func setStringDefault(dst *string, value string) {
	if strings.TrimSpace(*dst) == "" {
		*dst = strings.TrimSpace(value)
	}
}

func normalizeImportedHost(host *Host) {
	host.Name = strings.TrimSpace(host.Name)
	host.IP = strings.TrimSpace(host.IP)
	host.AuthRef = strings.TrimSpace(host.AuthRef)
	host.User = strings.TrimSpace(host.User)
	host.KeyPath = strings.TrimSpace(host.KeyPath)
	host.Remark = strings.TrimSpace(host.Remark)
	host.Group = strings.TrimSpace(host.Group)
	host.Jump = strings.TrimSpace(host.Jump)
	host.ConnectTimeout = strings.TrimSpace(host.ConnectTimeout)
	host.RunTimeout = strings.TrimSpace(host.RunTimeout)
	host.RemoteScriptDir = strings.TrimSpace(host.RemoteScriptDir)
	host.HostKeyCheck = strings.TrimSpace(host.HostKeyCheck)
	host.KnownHostsPath = strings.TrimSpace(host.KnownHostsPath)
	if host.Name == "" {
		host.Name = host.IP
	}
}

func validateImportedHost(host Host, line int) []HostImportError {
	var errs []HostImportError
	if strings.TrimSpace(host.IP) == "" {
		errs = append(errs, HostImportError{Line: line, Field: "ip", Message: "ip is required"})
	}
	if host.Port < 0 || host.Port > 65535 {
		errs = append(errs, HostImportError{Line: line, Field: "port", Message: fmt.Sprintf("invalid ssh port %d", host.Port)})
	}
	if value := strings.TrimSpace(host.HostKeyCheck); value != "" && value != HostKeyCheckKnownHosts && value != HostKeyCheckInsecure {
		errs = append(errs, HostImportError{Line: line, Field: "host_key_check", Message: fmt.Sprintf("invalid host_key_check %q, want known_hosts or insecure", value)})
	}
	return errs
}

func normalizeHostImportField(field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	switch field {
	case "host", "hostname":
		return "ip"
	case "auth":
		return "auth_ref"
	case "username":
		return "user"
	case "pwd":
		return "password"
	case "key", "keypath":
		return "key_path"
	case "jump_host":
		return "jump"
	default:
		return field
	}
}

func isHostImportField(field string) bool {
	switch field {
	case "name", "ip", "auth_ref", "user", "password", "key_path",
		"group", "remark", "port", "jump", "connect_timeout", "run_timeout",
		"remote_script_dir", "host_key_check", "known_hosts_path":
		return true
	default:
		return false
	}
}

func csvRecordIsBlank(record []string) bool {
	for _, value := range record {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
