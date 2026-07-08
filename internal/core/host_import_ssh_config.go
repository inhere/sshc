package core

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type SSHConfigImportOptions struct {
	Defaults           HostImportDefaults
	ImportIdentityFile bool
}

type SSHConfigImportWarning struct {
	Line    int
	Host    string
	Message string
}

func (w SSHConfigImportWarning) String() string {
	var parts []string
	if w.Line > 0 {
		parts = append(parts, fmt.Sprintf("line %d", w.Line))
	}
	if strings.TrimSpace(w.Host) != "" {
		parts = append(parts, fmt.Sprintf("host %q", w.Host))
	}
	if len(parts) == 0 {
		return w.Message
	}
	return strings.Join(parts, ": ") + ": " + w.Message
}

func ParseSSHConfigImport(reader io.Reader, opts SSHConfigImportOptions) ([]Host, []SSHConfigImportWarning, []HostImportError) {
	scanner := bufio.NewScanner(reader)
	var hosts []Host
	var warnings []SSHConfigImportWarning
	var errs []HostImportError
	var current *Host
	var currentLine int
	var identitySeen bool
	authDefault := strings.TrimSpace(opts.Defaults.AuthRef) != ""

	flush := func() {
		if current == nil {
			return
		}
		applyHostImportDefaults(current, opts.Defaults)
		normalizeImportedHost(current)
		if current.Port == 0 {
			current.Port = DefaultSSHPort
		}
		if rowErrs := validateImportedHost(*current, currentLine); len(rowErrs) > 0 {
			errs = append(errs, rowErrs...)
		} else {
			hosts = append(hosts, *current)
		}
		current = nil
		identitySeen = false
	}

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := trimSSHConfigLine(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.ToLower(fields[0])
		values := fields[1:]
		if key == "host" {
			flush()
			if len(values) != 1 || hasSSHHostPattern(values[0]) {
				current = nil
				identitySeen = false
				continue
			}
			current = &Host{Name: values[0]}
			currentLine = lineNo
			continue
		}
		if key == "match" {
			flush()
			current = nil
			warnings = append(warnings, SSHConfigImportWarning{Line: lineNo, Message: "Match blocks are not imported"})
			continue
		}
		if key == "include" {
			warnings = append(warnings, SSHConfigImportWarning{Line: lineNo, Message: "Include directives are not expanded"})
			continue
		}
		if current == nil {
			continue
		}
		value := strings.Join(values, " ")
		hostName := current.Name
		switch key {
		case "hostname":
			current.IP = value
		case "user":
			if !authDefault {
				current.User = value
			}
		case "port":
			port, err := strconv.Atoi(value)
			if err != nil || port < 1 || port > 65535 {
				errs = append(errs, HostImportError{Line: lineNo, Field: "port", Message: fmt.Sprintf("invalid ssh port %q", value)})
				continue
			}
			current.Port = port
		case "identityfile":
			if identitySeen {
				warnings = append(warnings, SSHConfigImportWarning{Line: lineNo, Host: hostName, Message: "multiple IdentityFile entries found; only the first is imported"})
				continue
			}
			identitySeen = true
			if !authDefault || opts.ImportIdentityFile {
				current.KeyPath = value
			}
		case "proxyjump":
			if strings.Contains(value, ",") {
				warnings = append(warnings, SSHConfigImportWarning{Line: lineNo, Host: hostName, Message: "multi-hop ProxyJump is not imported"})
				continue
			}
			current.Jump = value
		case "proxycommand":
			warnings = append(warnings, SSHConfigImportWarning{Line: lineNo, Host: hostName, Message: "ProxyCommand is not converted"})
		case "localforward", "remoteforward", "dynamicforward":
			warnings = append(warnings, SSHConfigImportWarning{Line: lineNo, Host: hostName, Message: key + " is ignored"})
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		errs = append(errs, HostImportError{Message: err.Error()})
	}
	return hosts, warnings, errs
}

func trimSSHConfigLine(raw string) string {
	line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}
	if idx := strings.Index(line, "#"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	return line
}
