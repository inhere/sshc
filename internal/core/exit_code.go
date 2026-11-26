package core

import "sync/atomic"

// pendingExitCode is the process exit code an interactive command wants when the
// command itself does not fail. ssh uses the remote shell status this way.
var pendingExitCode atomic.Int64

// SetExitCode records the exit code the CLI should end with.
func SetExitCode(code int) {
	pendingExitCode.Store(int64(code))
}

// TakeExitCode returns the recorded exit code and clears it.
func TakeExitCode() int {
	return int(pendingExitCode.Swap(0))
}
