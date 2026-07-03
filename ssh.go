package main

import (
	"time"

	"github.com/melbahja/goph"
	"golang.org/x/crypto/ssh"
)

func executeRemote(host Host, command string) ([]byte, error) {
	client, err := goph.NewConn(&goph.Config{
		User:     host.User,
		Addr:     host.IP,
		Port:     uint(host.Port),
		Auth:     goph.KeyboardInteractive(host.Password),
		Timeout:  20 * time.Second,
		Callback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return nil, err
	}
	defer client.Close()

	return client.Run(command)
}
