package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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

const (
	defaultRemoteKillAfter = 30 * time.Second
	clientTimeoutBuffer    = 5 * time.Second
)

type RunOptions struct {
	Timeout          time.Duration
	KillAfter        time.Duration
	Env              map[string]string
	CWD              string
	Sudo             bool
	SudoUser         string
	ScriptPath       string
	RemoteScriptPath string
	KeepRemoteScript bool
}

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
	SHA256 bool
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

func remoteTimeoutCommand(command string, opts RunOptions) string {
	if opts.Timeout <= 0 {
		return command
	}
	killAfter := effectiveKillAfter(opts.KillAfter)
	return "command -v timeout >/dev/null 2>&1 || { echo 'sshc: remote timeout command not found' >&2; exit 127; }; " +
		"timeout --kill-after=" + remoteDuration(killAfter) + " " + remoteDuration(opts.Timeout) + " bash -lc " + shellQuote(command)
}

func remoteSudoCommand(command string, opts RunOptions) string {
	if opts.SudoUser != "" {
		return "sudo -u " + shellQuote(opts.SudoUser) + " bash -lc " + shellQuote(command)
	}
	if opts.Sudo {
		return "sudo bash -lc " + shellQuote(command)
	}
	return command
}

func remoteClientTimeout(opts RunOptions) time.Duration {
	if opts.Timeout <= 0 {
		return 0
	}
	return opts.Timeout + effectiveKillAfter(opts.KillAfter) + clientTimeoutBuffer
}

func effectiveKillAfter(value time.Duration) time.Duration {
	if value <= 0 {
		return defaultRemoteKillAfter
	}
	return value
}

func remoteDuration(value time.Duration) string {
	return fmt.Sprintf("%gs", value.Seconds())
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

func UploadRemote(host Host, localPath, remotePath string, opts TransferOptions) (result TransferResult, err error) {
	started := time.Now()
	defer func() {
		result.Elapsed = time.Since(started)
	}()

	client, err := newSSHClient(host)
	if err != nil {
		return result, err
	}
	defer client.Close()

	info, err := os.Stat(localPath)
	if err != nil {
		return result, err
	}
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

func fileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func remoteSHA256(client *goph.Client, remotePath string) (string, error) {
	command := "command -v sha256sum >/dev/null 2>&1 || { echo 'sshc: remote sha256sum command not found' >&2; exit 127; }; sha256sum " + shellQuote(remotePath)
	out, err := client.Run(command)
	if err != nil {
		return "", err
	}
	return parseSHA256SumOutput(string(out))
}

func parseSHA256SumOutput(output string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty sha256sum output")
	}
	hash := fields[0]
	if len(hash) != sha256.Size*2 {
		return "", fmt.Errorf("invalid sha256sum output %q", strings.TrimSpace(output))
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return "", fmt.Errorf("invalid sha256sum output %q", strings.TrimSpace(output))
	}
	return strings.ToLower(hash), nil
}

func verifySHA256(localHash, remoteHash string) error {
	if localHash != remoteHash {
		return fmt.Errorf("sha256 mismatch: local=%s remote=%s", localHash, remoteHash)
	}
	return nil
}
