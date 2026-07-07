package core

import (
	"encoding/csv"
	"errors"
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
	Backend         string
	Via             string
	RunTemplate     string
	LoginCommand    string
	ConnectTimeout  string
	RunTimeout      string
	RemoteScriptDir string
	HostKeyCheck    string
	KnownHostsPath  string
}

type HostImportOptions struct {
	Format       HostImportFormat
	Defaults     HostImportDefaults
	Overwrite    bool
	SkipExisting bool
}

type HostImportAction string

const (
	HostImportActionAdd    HostImportAction = "add"
	HostImportActionUpdate HostImportAction = "update"
	HostImportActionSkip   HostImportAction = "skip"
)

type HostImportChange struct {
	Action        HostImportAction
	Host          Host
	ExistingIndex int
	Reason        string
}

type HostImportConflict struct {
	Host   Host
	Field  string
	Value  string
	Reason string
}

type HostImportPlan struct {
	Hosts     []Host
	Changes   []HostImportChange
	Added     int
	Updated   int
	Skipped   int
	Conflicts []HostImportConflict
	Invalid   []HostImportError
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

func ParseHostKV(text string, defaults HostImportDefaults) (Host, []HostImportError) {
	var lines []plainLine
	for idx, raw := range strings.Split(text, "\n") {
		lineNo := idx + 1
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, plainLine{Line: lineNo, Text: line})
	}
	if len(lines) == 0 {
		return Host{}, []HostImportError{{Message: "host fields are empty"}}
	}
	return parseHostImportPlainBlock(lines, defaults)
}

func PlanHostImport(config Config, hosts []Host, opts HostImportOptions) (HostImportPlan, error) {
	plan := HostImportPlan{Hosts: append([]Host(nil), hosts...)}
	if opts.SkipExisting && opts.Overwrite {
		plan.Invalid = append(plan.Invalid, HostImportError{Message: "skip-existing and overwrite cannot be used together"})
		return plan, errors.New("host import options are invalid")
	}
	plan.Invalid = append(plan.Invalid, validateHostImportInputDuplicates(hosts)...)
	if len(plan.Invalid) > 0 {
		return plan, errors.New("host import input has errors")
	}

	for _, host := range hosts {
		nameIdx := findExistingHostByName(config.Hosts, host.Name)
		ipIdx := findExistingHostByIP(config.Hosts, host.IP)
		existingIdx := mergeExistingIndexes(nameIdx, ipIdx)
		if existingIdx == -2 {
			plan.Conflicts = append(plan.Conflicts, HostImportConflict{
				Host:   host,
				Field:  "name/ip",
				Value:  fmt.Sprintf("%s/%s", host.Name, host.IP),
				Reason: "name and ip match different existing hosts",
			})
			continue
		}

		if existingIdx >= 0 {
			if opts.SkipExisting {
				plan.Changes = append(plan.Changes, HostImportChange{
					Action:        HostImportActionSkip,
					Host:          host,
					ExistingIndex: existingIdx,
					Reason:        "existing host",
				})
				plan.Skipped++
				continue
			}
			if opts.Overwrite {
				if err := validatePlannedHost(config, host); err != nil {
					plan.Invalid = append(plan.Invalid, HostImportError{Field: "host", Message: err.Error()})
					continue
				}
				plan.Changes = append(plan.Changes, HostImportChange{
					Action:        HostImportActionUpdate,
					Host:          host,
					ExistingIndex: existingIdx,
				})
				plan.Updated++
				continue
			}
			if nameIdx >= 0 {
				plan.Conflicts = append(plan.Conflicts, HostImportConflict{Host: host, Field: "name", Value: host.Name, Reason: "host name already exists"})
			}
			if ipIdx >= 0 {
				plan.Conflicts = append(plan.Conflicts, HostImportConflict{Host: host, Field: "ip", Value: host.IP, Reason: "host ip already exists"})
			}
			continue
		}

		if err := validatePlannedHost(config, host); err != nil {
			plan.Invalid = append(plan.Invalid, HostImportError{Field: "host", Message: err.Error()})
			continue
		}
		plan.Changes = append(plan.Changes, HostImportChange{Action: HostImportActionAdd, Host: host, ExistingIndex: -1})
		plan.Added++
	}

	if len(plan.Invalid) > 0 || len(plan.Conflicts) > 0 {
		return plan, errors.New("host import plan has conflicts")
	}
	return plan, nil
}

func ApplyHostImport(config *Config, plan HostImportPlan) error {
	if config == nil {
		return errors.New("config is nil")
	}
	if len(plan.Invalid) > 0 || len(plan.Conflicts) > 0 {
		return errors.New("host import plan has conflicts")
	}
	hosts := append([]Host(nil), config.Hosts...)
	for _, change := range plan.Changes {
		switch change.Action {
		case HostImportActionAdd:
			hosts = append(hosts, change.Host)
		case HostImportActionUpdate:
			if change.ExistingIndex < 0 || change.ExistingIndex >= len(hosts) {
				return fmt.Errorf("host import update index %d out of range", change.ExistingIndex)
			}
			hosts[change.ExistingIndex] = change.Host
		case HostImportActionSkip:
			continue
		default:
			return fmt.Errorf("unknown host import action %q", change.Action)
		}
	}
	config.Hosts = hosts
	return nil
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
	case "backend":
		host.Backend = value
	case "via":
		host.Via = value
	case "run_template":
		host.RunTemplate = value
	case "login_command":
		host.LoginCommand = value
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
	setStringDefault(&host.Backend, defaults.Backend)
	setStringDefault(&host.Via, defaults.Via)
	setStringDefault(&host.RunTemplate, defaults.RunTemplate)
	setStringDefault(&host.LoginCommand, defaults.LoginCommand)
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
	NormalizeHostFields(host)
	if host.Name == "" {
		host.Name = host.IP
	}
}

func validateImportedHost(host Host, line int) []HostImportError {
	var errs []HostImportError
	if strings.TrimSpace(host.IP) == "" && !IsCommandProxyHost(host) {
		errs = append(errs, HostImportError{Line: line, Field: "ip", Message: "ip is required"})
	}
	if host.Port < 0 || host.Port > 65535 {
		errs = append(errs, HostImportError{Line: line, Field: "port", Message: fmt.Sprintf("invalid ssh port %d", host.Port)})
	}
	if value := strings.TrimSpace(host.HostKeyCheck); value != "" && value != HostKeyCheckKnownHosts && value != HostKeyCheckInsecure {
		errs = append(errs, HostImportError{Line: line, Field: "host_key_check", Message: fmt.Sprintf("invalid host_key_check %q, want known_hosts or insecure", value)})
	}
	if err := validateHostBackend(host); err != nil {
		errs = append(errs, HostImportError{Line: line, Field: "backend", Message: err.Error()})
	}
	if IsCommandProxyHost(host) {
		if strings.TrimSpace(host.Name) == "" {
			errs = append(errs, HostImportError{Line: line, Field: "name", Message: "name is required for command_proxy host"})
		}
		if strings.TrimSpace(host.Via) == "" {
			errs = append(errs, HostImportError{Line: line, Field: "via", Message: "via is required for command_proxy host"})
		}
		if strings.TrimSpace(host.RunTemplate) == "" && strings.TrimSpace(host.LoginCommand) == "" {
			errs = append(errs, HostImportError{Line: line, Field: "run_template", Message: "run_template or login_command is required for command_proxy host"})
		}
		if err := ValidateCommandProxyTemplate(host); err != nil {
			errs = append(errs, HostImportError{Line: line, Field: "run_template", Message: err.Error()})
		}
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
	case "key", "keypath", "keyfile":
		return "key_path"
	case "jump_host":
		return "jump"
	case "run-template":
		return "run_template"
	case "login-command":
		return "login_command"
	default:
		return field
	}
}

func isHostImportField(field string) bool {
	switch field {
	case "name", "ip", "auth_ref", "user", "password", "key_path",
		"group", "remark", "port", "jump", "backend", "via", "run_template", "login_command", "connect_timeout", "run_timeout",
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

func validateHostImportInputDuplicates(hosts []Host) []HostImportError {
	var errs []HostImportError
	seenNames := map[string]struct{}{}
	seenIPs := map[string]struct{}{}
	for _, host := range hosts {
		if name := strings.TrimSpace(host.Name); name != "" {
			key := strings.ToLower(name)
			if _, ok := seenNames[key]; ok {
				errs = append(errs, HostImportError{Field: "name", Message: fmt.Sprintf("duplicate imported host name %q", name)})
			}
			seenNames[key] = struct{}{}
		}
		if ip := strings.TrimSpace(host.IP); ip != "" {
			key := strings.ToLower(ip)
			if _, ok := seenIPs[key]; ok {
				errs = append(errs, HostImportError{Field: "ip", Message: fmt.Sprintf("duplicate imported host ip %q", ip)})
			}
			seenIPs[key] = struct{}{}
		}
	}
	return errs
}

func validatePlannedHost(config Config, host Host) error {
	if _, _, err := config.EffectiveHost(host, HostOverrides{}); err != nil {
		return err
	}
	if IsCommandProxyHost(host) {
		checkConfig := config
		checkConfig.Hosts = append(append([]Host(nil), config.Hosts...), host)
		for _, issue := range CheckConfig(checkConfig) {
			if issue.Level == DoctorError && strings.Contains(issue.Message, HostLogName(host)) {
				return errors.New(issue.Message)
			}
		}
	}
	if jump := strings.TrimSpace(host.Jump); jump != "" {
		if _, err := config.ResolveConnection(host); err != nil {
			return err
		}
	}
	return nil
}

func findExistingHostByName(hosts []Host, name string) int {
	name = strings.TrimSpace(name)
	if name == "" {
		return -1
	}
	for i, host := range hosts {
		if strings.TrimSpace(host.Name) == name {
			return i
		}
	}
	return -1
}

func findExistingHostByIP(hosts []Host, ip string) int {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return -1
	}
	for i, host := range hosts {
		if strings.TrimSpace(host.IP) == ip {
			return i
		}
	}
	return -1
}

func mergeExistingIndexes(a, b int) int {
	switch {
	case a >= 0 && b >= 0 && a != b:
		return -2
	case a >= 0:
		return a
	default:
		return b
	}
}
