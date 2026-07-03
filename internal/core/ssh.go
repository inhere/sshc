package core

import (
	"context"
	"crypto/rand"
	"fmt"
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

type RunOptions struct {
	Timeout          time.Duration
	KillAfter        time.Duration
	Env              map[string]string
	CWD              string
	ScriptPath       string
	RemoteScriptPath string
	KeepRemoteScript bool
}

func ExecuteRemote(host Host, command string, opts RunOptions) ([]byte, error) {
	client, err := newSSHClient(host)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	if opts.ScriptPath != "" {
		return executeRemoteScript(client, opts)
	}

	remoteCommand, err := BuildRemoteCommandWithCWD(command, opts.Env, opts.CWD)
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

func executeRemoteScript(client *goph.Client, opts RunOptions) ([]byte, error) {
	remoteScriptPath := opts.RemoteScriptPath
	if remoteScriptPath == "" {
		remoteScriptPath = NewRemoteScriptPath(Now())
	}
	if err := uploadRemoteScript(client, opts.ScriptPath, remoteScriptPath); err != nil {
		return nil, err
	}
	if !opts.KeepRemoteScript {
		defer func() {
			_, _ = client.Run("rm -f " + shellQuote(remoteScriptPath))
		}()
	}

	if _, err := client.Run("chmod 700 " + shellQuote(remoteScriptPath)); err != nil {
		return nil, err
	}
	remoteCommand, err := BuildRemoteCommandWithCWD(scriptExecuteCommand(remoteScriptPath), opts.Env, opts.CWD)
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

func uploadRemoteScript(client *goph.Client, localPath, remotePath string) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("script path %q is a directory", localPath)
	}

	sftpClient, err := client.NewSftp()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	if err := mkdirRemoteParent(sftpClient, remotePath); err != nil {
		return err
	}
	return uploadFileWithSFTP(sftpClient, localPath, remotePath)
}

func scriptExecuteCommand(remotePath string) string {
	return "bash " + shellQuote(remotePath)
}

func NewRemoteScriptPath(value time.Time) string {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Sprintf("/tmp/sshc-run-%d.sh", value.UnixNano())
	}
	return fmt.Sprintf("/tmp/sshc-run-%d-%x.sh", value.UnixNano(), suffix[:])
}

func UploadRemote(host Host, localPath, remotePath string) error {
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
		remoteFile := RemoteFilePath(localPath, remotePath)
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
			remoteCurrent = JoinRemotePath(remotePath, filepath.ToSlash(rel))
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

func FetchRemote(host Host, remotePath, localPath string) error {
	client, err := newSSHClient(host)
	if err != nil {
		return err
	}
	defer client.Close()

	sftpClient, err := client.NewSftp()
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	info, err := sftpClient.Stat(remotePath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return downloadFileWithSFTP(sftpClient, remotePath, LocalFilePath(remotePath, localPath))
	}

	root := strings.TrimRight(remotePath, "/")
	localRoot := LocalDirPath(root, localPath)
	walker := sftpClient.Walk(root)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return err
		}

		remoteCurrent := walker.Path()
		localCurrent := localRoot
		if rel := RemoteRelPath(root, remoteCurrent); rel != "" {
			localCurrent = filepath.Join(localRoot, filepath.FromSlash(rel))
		}
		if walker.Stat().IsDir() {
			if err := os.MkdirAll(localCurrent, 0700); err != nil {
				return err
			}
			continue
		}
		if err := downloadFileWithSFTP(sftpClient, remoteCurrent, localCurrent); err != nil {
			return err
		}
	}
	return nil
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

func RemoteFilePath(localPath, remotePath string) string {
	if strings.HasSuffix(remotePath, "/") {
		return JoinRemotePath(remotePath, filepath.Base(localPath))
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

func downloadFileWithSFTP(client *sftp.Client, remotePath, localPath string) error {
	remoteFile, err := client.Open(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
		return err
	}
	localFile, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	_, err = io.Copy(localFile, remoteFile)
	return err
}

func LocalFilePath(remotePath, localPath string) string {
	if isLocalDirTarget(localPath) {
		return filepath.Join(localPath, path.Base(strings.TrimRight(remotePath, "/")))
	}
	return localPath
}

func LocalDirPath(remotePath, localPath string) string {
	if isLocalDirTarget(localPath) {
		return filepath.Join(localPath, path.Base(strings.TrimRight(remotePath, "/")))
	}
	return localPath
}

func isLocalDirTarget(localPath string) bool {
	if strings.HasSuffix(localPath, "/") || strings.HasSuffix(localPath, `\`) {
		return true
	}
	info, err := os.Stat(localPath)
	return err == nil && info.IsDir()
}

func RemoteRelPath(root, current string) string {
	root = strings.TrimRight(root, "/")
	current = strings.TrimRight(current, "/")
	if current == root {
		return ""
	}
	return strings.TrimPrefix(strings.TrimPrefix(current, root), "/")
}

func JoinRemotePath(base, elem string) string {
	if base == "" || base == "." {
		return elem
	}
	if strings.HasSuffix(base, "/") {
		return path.Clean(base + elem)
	}
	return path.Join(base, elem)
}
