//go:build windows

package core

import "golang.org/x/crypto/ssh"

func startPTYResizeLoop(fd int, session *ssh.Session) func() {
	return func() {}
}
