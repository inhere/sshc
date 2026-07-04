package command

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/gookit/gcli/v3"
	"github.com/inhere/sshc/internal/core"
)

var commandOutput io.Writer = os.Stdout

func cmdOutput(_ *gcli.Command) io.Writer {
	if commandOutput == nil {
		return os.Stdout
	}
	return commandOutput
}

func setCommandOutputForTest(out io.Writer) func() {
	old := commandOutput
	commandOutput = out
	return func() { commandOutput = old }
}

func resolveCommandHost(target string) (core.Host, error) {
	return resolveCommandHostWithOptions(target, core.ResolveConnectionOptions{})
}

func resolveCommandHostWithOptions(target string, opts core.ResolveConnectionOptions) (core.Host, error) {
	host, ok, err := core.ResolveHostWithSSHConfig(target, core.HostOverrides{})
	if err != nil {
		return core.Host{}, err
	}
	if !ok {
		return core.Host{}, fmt.Errorf("host %q not found", target)
	}
	if jump := strings.TrimSpace(opts.Jump); jump != "" {
		host.Jump = jump
	}
	return host, nil
}

func parseClipboardHost(text string) (core.Host, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return core.Host{}, fmt.Errorf("clipboard is empty")
	}
	var host core.Host
	if strings.Contains(text, "=") {
		host = parseClipboardKeyValues(text)
	} else {
		fields := strings.Split(text, ",")
		if len(fields) < 3 || len(fields) > 5 {
			return host, fmt.Errorf("clipboard must be key=value lines or ip,user,password,name,port")
		}
		host.IP = strings.TrimSpace(fields[0])
		host.User = strings.TrimSpace(fields[1])
		host.Password = strings.TrimSpace(fields[2])
		if len(fields) >= 4 {
			host.Name = strings.TrimSpace(fields[3])
		}
		if len(fields) >= 5 && strings.TrimSpace(fields[4]) != "" {
			port, err := strconv.Atoi(strings.TrimSpace(fields[4]))
			if err != nil {
				return host, fmt.Errorf("invalid ssh port %q", strings.TrimSpace(fields[4]))
			}
			host.Port = port
		}
	}
	if host.Port == 0 {
		host.Port = core.DefaultSSHPort
	}
	normalizeHostDefaults(&host)
	return host, nil
}

func parseClipboardKeyValues(text string) core.Host {
	host := core.Host{}
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			key, value, ok = strings.Cut(line, ":")
			if !ok {
				continue
			}
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "ip", "host", "hostname":
			host.IP = value
		case "name":
			host.Name = value
		case "user", "username":
			host.User = value
		case "password", "pwd":
			host.Password = value
		case "key", "key_path", "keypath":
			host.KeyPath = value
		case "jump", "jump_host":
			host.Jump = value
		case "remark":
			host.Remark = value
		case "group":
			host.Group = value
		case "port":
			if port, err := strconv.Atoi(value); err == nil {
				host.Port = port
			}
		}
	}
	return host
}

func readSystemClipboard() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw")
	case "darwin":
		cmd = exec.Command("pbpaste")
	default:
		if _, err := exec.LookPath("wl-paste"); err == nil {
			cmd = exec.Command("wl-paste", "--no-newline")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else {
			return "", fmt.Errorf("no clipboard reader found; install wl-paste or xclip")
		}
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
