package command

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	if clipboardLooksKV(text) {
		parsed, errs := core.ParseHostKV(text, core.HostImportDefaults{Group: core.DefaultGroup, Port: core.DefaultSSHPort})
		if len(errs) > 0 {
			return host, fmt.Errorf("invalid clipboard host: %s", errs[0].Error())
		}
		host = parsed
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

func normalizeKeyPathForSave(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || strings.HasPrefix(path, "~") || filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

func clipboardLooksKV(text string) bool {
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, _, ok := strings.Cut(line, "="); ok {
			return true
		}
		if key, _, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(key) != "" && !strings.Contains(key, ",") {
			return true
		}
	}
	return false
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
