//go:build windows

package core

import "os/exec"

// secretShellCommand picks the first available Windows shell: pwsh, then
// windows powershell, then cmd.
func secretShellCommand(command string) (string, []string) {
	for _, name := range []string{"pwsh", "powershell"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, []string{"-NoProfile", "-Command", command}
		}
	}
	if path, err := exec.LookPath("cmd"); err == nil {
		return path, []string{"/C", command}
	}
	return "", nil
}
