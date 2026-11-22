//go:build !windows

package core

// secretShellCommand runs the command through the POSIX shell.
func secretShellCommand(command string) (string, []string) {
	return "/bin/sh", []string{"-c", command}
}
