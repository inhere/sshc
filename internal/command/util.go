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

const keyPassphraseEnvKey = "SSHC_KEY_PASSPHRASE"

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

type keyPassphraseFlag struct {
	Source  string
	SetFlag bool
}

func (f *keyPassphraseFlag) Set(value string) error {
	value = normalizeKeyPassphraseSource(value)
	if value == "" || value == "true" {
		value = "input"
	}
	if value == "false" {
		f.Source = ""
		f.SetFlag = false
		return nil
	}
	f.Source = value
	f.SetFlag = true
	return nil
}

func (f *keyPassphraseFlag) String() string {
	return f.Source
}

func (f *keyPassphraseFlag) IsBoolFlag() bool {
	return true
}

func consumeKeyPassphraseSourceArg(flag *keyPassphraseFlag, args []string) ([]string, error) {
	if flag == nil || !flag.SetFlag || flag.Source != "input" || len(args) == 0 {
		return args, nil
	}
	source := normalizeKeyPassphraseSource(args[0])
	if !isKeyPassphraseSource(source) {
		return nil, fmt.Errorf("invalid --key-passphrase source %q, want input, clip, or env", args[0])
	}
	if err := flag.Set(source); err != nil {
		return nil, err
	}
	return args[1:], nil
}

func resolveKeyPassphrase(flag keyPassphraseFlag) (string, error) {
	if !flag.SetFlag {
		return "", nil
	}
	source := normalizeKeyPassphraseSource(flag.Source)
	if source == "" {
		source = "input"
	}
	switch source {
	case "input":
		return strings.TrimSpace(readInteractivePassword("Key passphrase: ")), nil
	case "clip":
		text, err := readClipboard()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(text), nil
	case "env":
		value := strings.TrimSpace(os.Getenv(keyPassphraseEnvKey))
		if value == "" {
			return "", fmt.Errorf("%s is empty", keyPassphraseEnvKey)
		}
		return value, nil
	default:
		return "", fmt.Errorf("invalid --key-passphrase source %q, want input, clip, or env", flag.Source)
	}
}

func readKeyFileContent(keyPath string) (string, error) {
	keyPath = strings.TrimSpace(keyPath)
	if keyPath == "" {
		return "", fmt.Errorf("--embed-key requires --key")
	}
	path := core.ExpandUserPath(keyPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read key file %s: %w", path, err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("key file %s is empty", path)
	}
	return string(data), nil
}

func normalizeKeyPassphraseSource(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isKeyPassphraseSource(value string) bool {
	switch value {
	case "input", "clip", "env":
		return true
	default:
		return false
	}
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
