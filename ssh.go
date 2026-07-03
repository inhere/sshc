package main

import (
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/melbahja/goph"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func executeRemote(host Host, command string, opts RunOptions) ([]byte, error) {
	client, err := newSSHClient(host)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	remoteCommand, err := buildRemoteCommand(command, opts.Env)
	if err != nil {
		return nil, err
	}
	if opts.Timeout <= 0 {
		return client.Run(remoteCommand)
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()
	return client.RunContext(ctx, remoteCommand)
}

func uploadRemote(host Host, localPath, remotePath string) error {
	client, err := newSSHClient(host)
	if err != nil {
		return err
	}
	defer client.Close()

	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	sftpClient, err := client.NewSftp()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	if !info.IsDir() {
		remoteFile := remoteFilePath(localPath, remotePath)
		if err := mkdirRemoteParent(sftpClient, remoteFile); err != nil {
			return err
		}
		return uploadFileWithSFTP(sftpClient, localPath, remoteFile)
	}

	root := filepath.Clean(localPath)
	return filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}

		remoteCurrent := remotePath
		if rel != "." {
			remoteCurrent = joinRemotePath(remotePath, filepath.ToSlash(rel))
		}
		if entry.IsDir() {
			return sftpClient.MkdirAll(remoteCurrent)
		}

		if err := mkdirRemoteParent(sftpClient, remoteCurrent); err != nil {
			return err
		}
		return uploadFileWithSFTP(sftpClient, current, remoteCurrent)
	})
}

func newSSHClient(host Host) (*goph.Client, error) {
	return goph.NewConn(&goph.Config{
		User:     host.User,
		Addr:     host.IP,
		Port:     uint(host.Port),
		Auth:     goph.KeyboardInteractive(host.Password),
		Timeout:  20 * time.Second,
		Callback: ssh.InsecureIgnoreHostKey(),
	})
}

func remoteFilePath(localPath, remotePath string) string {
	if strings.HasSuffix(remotePath, "/") {
		return joinRemotePath(remotePath, filepath.Base(localPath))
	}
	return remotePath
}

func mkdirRemoteParent(client *sftp.Client, remotePath string) error {
	parent := path.Dir(remotePath)
	if parent == "." || parent == "/" {
		return nil
	}
	return client.MkdirAll(parent)
}

func uploadFileWithSFTP(client *sftp.Client, localPath, remotePath string) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	remoteFile, err := client.Create(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	_, err = io.Copy(remoteFile, localFile)
	return err
}

func joinRemotePath(base, elem string) string {
	if base == "" || base == "." {
		return elem
	}
	if strings.HasSuffix(base, "/") {
		return path.Clean(base + elem)
	}
	return path.Join(base, elem)
}
