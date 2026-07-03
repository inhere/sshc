//go:build !windows

package core

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh"
)

func startPTYResizeLoop(fd int, session *ssh.Session) func() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)
	done := make(chan struct{})

	go func() {
		for {
			select {
			case <-signals:
				width, height := loginTerminalSize(fd)
				_ = session.WindowChange(height, width)
			case <-done:
				return
			}
		}
	}()

	width, height := loginTerminalSize(fd)
	_ = session.WindowChange(height, width)
	return func() {
		signal.Stop(signals)
		close(done)
	}
}
