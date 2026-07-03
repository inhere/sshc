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
	"golang.org/x/term"
)

const (
	defaultRemoteKillAfter = 30 * time.Second
	clientTimeoutBuffer    = 5 * time.Second
)

type TransferResult struct {
	Bytes        int64
	Files        int
	Directories  int
	Elapsed      time.Duration
	LocalSHA256  string
	RemoteSHA256 string
	SHA256OK     bool
}

type TransferOptions struct {
	SHA256    bool
	RemoveDir bool
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
	remoteCommand = remoteSudoCommand(remoteCommand, opts)
	remoteCommand = remoteTimeoutCommand(remoteCommand, opts)
	clientTimeout := remoteClientTimeout(opts)
	if clientTimeout <= 0 {
		return client.Run(remoteCommand)
	}

	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()
	return client.RunContext(ctx, remoteCommand)
}

func LoginRemote(host Host) error {
	client, err := newSSHClient(host)
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	fd := int(os.Stdin.Fd())
	width, height := 120, 40
	if term.IsTerminal(fd) {
		if w, h, err := term.GetSize(fd); err == nil {
			width, height = w, h
		}
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return err
		}
		defer func() {
			_ = term.Restore(fd, oldState)
		}()
	}

	if err := session.RequestPty("xterm-256color", height, width, ssh.TerminalModes{}); err != nil {
		return err
	}
	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	if err := session.Shell(); err != nil {
		return err
	}
	return session.Wait()
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
	remoteCommand = remoteSudoCommand(remoteCommand, opts)
	remoteCommand = remoteTimeoutCommand(remoteCommand, opts)
	clientTimeout := remoteClientTimeout(opts)
	if clientTimeout <= 0 {
		return client.Run(remoteCommand)
	}

	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
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
	_, err = uploadFileWithSFTP(sftpClient, localPath, remotePath)
	return err
}

func NewRemoteScriptPath(value time.Time) string {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Sprintf("/tmp/sshc-run-%d.sh", value.UnixNano())
	}
	return fmt.Sprintf("/tmp/sshc-run-%d-%x.sh", value.UnixNano(), suffix[:])
}

func UploadRemote(host Host, localPath, remotePath string, opts TransferOptions) (result TransferResult, err error) {
	started := time.Now()
	defer func() {
		result.Elapsed = time.Since(started)
	}()

	if isLocalGlob(localPath) {
		return uploadRemoteGlob(host, localPath, remotePath, opts, started)
	}

	info, err := os.Stat(localPath)
	if err != nil {
		return result, err
	}
	if !info.IsDir() && opts.RemoveDir {
		return result, fmt.Errorf("--remove-dir is only supported for directory uploads")
	}

	client, err := newSSHClient(host)
	if err != nil {
		return result, err
	}
	defer client.Close()

	sftpClient, err := client.NewSftp()
	if err != nil {
		return result, err
	}
	defer sftpClient.Close()

	if !info.IsDir() {
		remoteFile := RemoteFilePath(localPath, remotePath)
		if err := mkdirRemoteParent(sftpClient, remoteFile); err != nil {
			return result, err
		}
		if opts.SHA256 {
			result.LocalSHA256, err = fileSHA256(localPath)
			if err != nil {
				return result, err
			}
		}
		bytes, err := uploadFileWithSFTP(sftpClient, localPath, remoteFile)
		if err != nil {
			return result, err
		}
		result.Bytes += bytes
		result.Files++
		if opts.SHA256 {
			result.RemoteSHA256, err = remoteSHA256(client, remoteFile)
			if err != nil {
				return result, err
			}
			if err := verifySHA256(result.LocalSHA256, result.RemoteSHA256); err != nil {
				return result, err
			}
			result.SHA256OK = true
		}
		return result, nil
	}
	if opts.SHA256 {
		return result, fmt.Errorf("--sha256 is only supported for file transfers")
	}
	if opts.RemoveDir {
		if err := validateRemoteRemoveDirPath(remotePath); err != nil {
			return result, err
		}
		if _, err := client.Run("rm -rf -- " + shellQuote(remotePath)); err != nil {
			return result, err
		}
	}

	root := filepath.Clean(localPath)
	err = filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
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
			result.Directories++
			return sftpClient.MkdirAll(remoteCurrent)
		}

		if err := mkdirRemoteParent(sftpClient, remoteCurrent); err != nil {
			return err
		}
		bytes, err := uploadFileWithSFTP(sftpClient, current, remoteCurrent)
		if err != nil {
			return err
		}
		result.Bytes += bytes
		result.Files++
		return nil
	})
	return result, err
}

func uploadRemoteGlob(host Host, pattern, remotePath string, opts TransferOptions, started time.Time) (result TransferResult, err error) {
	defer func() {
		result.Elapsed = time.Since(started)
	}()
	if opts.SHA256 {
		return result, fmt.Errorf("--sha256 is only supported for single file transfers")
	}
	if opts.RemoveDir {
		return result, fmt.Errorf("--remove-dir is only supported for directory uploads")
	}

	matches, err := expandLocalGlob(pattern)
	if err != nil {
		return result, err
	}

	client, err := newSSHClient(host)
	if err != nil {
		return result, err
	}
	defer client.Close()

	sftpClient, err := client.NewSftp()
	if err != nil {
		return result, err
	}
	defer sftpClient.Close()

	if err := sftpClient.MkdirAll(remotePath); err != nil {
		return result, err
	}
	for _, localFile := range matches {
		remoteFile := JoinRemotePath(remotePath, filepath.Base(localFile))
		bytes, err := uploadFileWithSFTP(sftpClient, localFile, remoteFile)
		if err != nil {
			return result, err
		}
		result.Bytes += bytes
		result.Files++
	}
	return result, nil
}

func FetchRemote(host Host, remotePath, localPath string, opts TransferOptions) (result TransferResult, err error) {
	started := time.Now()
	defer func() {
		result.Elapsed = time.Since(started)
	}()

	client, err := newSSHClient(host)
	if err != nil {
		return result, err
	}
	defer client.Close()

	sftpClient, err := client.NewSftp()
	if err != nil {
		return result, err
	}
	defer sftpClient.Close()

	info, err := sftpClient.Stat(remotePath)
	if err != nil {
		return result, err
	}
	if !info.IsDir() {
		localFile := LocalFilePath(remotePath, localPath)
		if opts.SHA256 {
			result.RemoteSHA256, err = remoteSHA256(client, remotePath)
			if err != nil {
				return result, err
			}
		}
		bytes, err := downloadFileWithSFTP(sftpClient, remotePath, localFile)
		if err != nil {
			return result, err
		}
		result.Bytes += bytes
		result.Files++
		if opts.SHA256 {
			result.LocalSHA256, err = fileSHA256(localFile)
			if err != nil {
				return result, err
			}
			if err := verifySHA256(result.LocalSHA256, result.RemoteSHA256); err != nil {
				return result, err
			}
			result.SHA256OK = true
		}
		return result, nil
	}
	if opts.SHA256 {
		return result, fmt.Errorf("--sha256 is only supported for file transfers")
	}

	root := strings.TrimRight(remotePath, "/")
	localRoot := LocalDirPath(root, localPath)
	walker := sftpClient.Walk(root)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return result, err
		}

		remoteCurrent := walker.Path()
		localCurrent := localRoot
		if rel := RemoteRelPath(root, remoteCurrent); rel != "" {
			localCurrent = filepath.Join(localRoot, filepath.FromSlash(rel))
		}
		if walker.Stat().IsDir() {
			result.Directories++
			if err := os.MkdirAll(localCurrent, 0700); err != nil {
				return result, err
			}
			continue
		}
		bytes, err := downloadFileWithSFTP(sftpClient, remoteCurrent, localCurrent)
		if err != nil {
			return result, err
		}
		result.Bytes += bytes
		result.Files++
	}
	return result, nil
}

func newSSHClient(host Host) (*goph.Client, error) {
	auth, err := hostAuth(host)
	if err != nil {
		return nil, err
	}
	return goph.NewConn(&goph.Config{
		User:     host.User,
		Addr:     host.IP,
		Port:     uint(host.Port),
		Auth:     auth,
		Timeout:  20 * time.Second,
		Callback: ssh.InsecureIgnoreHostKey(),
	})
}

func hostAuth(host Host) (goph.Auth, error) {
	var auth goph.Auth
	if strings.TrimSpace(host.KeyPath) != "" {
		keyAuth, err := goph.Key(expandUserPath(host.KeyPath), "")
		if err != nil {
			return nil, err
		}
		auth = append(auth, keyAuth...)
	}
	if host.Password != "" {
		auth = append(auth, goph.KeyboardInteractive(host.Password)...)
	}
	if len(auth) == 0 {
		return nil, fmt.Errorf("password or key_path is required")
	}
	return auth, nil
}

func mkdirRemoteParent(client *sftp.Client, remotePath string) error {
	parent := path.Dir(remotePath)
	if parent == "." || parent == "/" {
		return nil
	}
	return client.MkdirAll(parent)
}

func uploadFileWithSFTP(client *sftp.Client, localPath, remotePath string) (int64, error) {
	localFile, err := os.Open(localPath)
	if err != nil {
		return 0, err
	}
	defer localFile.Close()

	remoteFile, err := client.Create(remotePath)
	if err != nil {
		return 0, err
	}
	defer remoteFile.Close()

	return io.Copy(remoteFile, localFile)
}

func downloadFileWithSFTP(client *sftp.Client, remotePath, localPath string) (int64, error) {
	remoteFile, err := client.Open(remotePath)
	if err != nil {
		return 0, err
	}
	defer remoteFile.Close()

	if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
		return 0, err
	}
	localFile, err := os.Create(localPath)
	if err != nil {
		return 0, err
	}
	defer localFile.Close()

	return io.Copy(localFile, remoteFile)
}

func remoteSHA256(client *goph.Client, remotePath string) (string, error) {
	command := "command -v sha256sum >/dev/null 2>&1 || { echo 'sshc: remote sha256sum command not found' >&2; exit 127; }; sha256sum " + shellQuote(remotePath)
	out, err := client.Run(command)
	if err != nil {
		return "", err
	}
	return parseSHA256SumOutput(string(out))
}
