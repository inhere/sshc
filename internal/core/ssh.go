package core

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/melbahja/goph"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

const (
	defaultRemoteKillAfter = 30 * time.Second
	clientTimeoutBuffer    = 5 * time.Second
	defaultPTYWidth        = 120
	defaultPTYHeight       = 40
	defaultPTYTerm         = "xterm-256color"
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

type TransferJob struct {
	LocalPath  string
	RemotePath string
	RemoteDir  bool
}

type LoginOptions struct {
	Stdin  *os.File
	Stdout io.Writer
	Stderr io.Writer
	Term   string
}

type RemoteClient interface {
	Run(string) ([]byte, error)
	RunContext(context.Context, string) ([]byte, error)
	NewSession() (*ssh.Session, error)
	NewSftp(...sftp.ClientOption) (*sftp.Client, error)
	Close() error
}

type remoteClient struct {
	*goph.Client
	closeAll func() error
	dial     func(network, addr string) (net.Conn, error)
}

func (client *remoteClient) Close() error {
	if client.closeAll != nil {
		return client.closeAll()
	}
	if client.Client == nil {
		return nil
	}
	return client.Client.Close()
}

func (client *remoteClient) Dial(network, addr string) (net.Conn, error) {
	if remoteClientDialForTest != nil {
		return remoteClientDialForTest(client, network, addr)
	}
	if client.dial != nil {
		return client.dial(network, addr)
	}
	return client.Client.Dial(network, addr)
}

var (
	newGophConn             = goph.NewConn
	newSSHClientConn        = ssh.NewClientConn
	remoteClientDialForTest func(*remoteClient, string, string) (net.Conn, error)
)

func ExecuteRemote(host Host, command string, opts RunOptions) ([]byte, error) {
	if IsCommandProxyHost(host) {
		return ExecuteCommandProxy(host, command, opts)
	}
	client, err := newSSHClient(host)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	if opts.ScriptPath != "" {
		return executeRemoteScript(client, opts)
	}

	remoteCommand, err := BuildRemoteRunCommand(command, opts)
	if err != nil {
		return nil, err
	}
	clientTimeout := remoteClientTimeout(opts)
	if clientTimeout <= 0 {
		return client.Run(remoteCommand)
	}

	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()
	return client.RunContext(ctx, remoteCommand)
}

func LoginRemote(host Host) error {
	return LoginRemoteWithOptions(host, LoginOptions{})
}

func LoginRemoteWithOptions(host Host, opts LoginOptions) error {
	opts = normalizeLoginOptions(opts)
	fd := int(opts.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("interactive login requires terminal stdin")
	}
	width, height := loginTerminalSize(fd)

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

	if err := session.RequestPty(loginTermName(opts.Term), height, width, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		return err
	}
	session.Stdin = opts.Stdin
	session.Stdout = opts.Stdout
	session.Stderr = opts.Stderr

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer func() {
		_ = term.Restore(fd, oldState)
	}()
	stopResize := startPTYResizeLoop(fd, session)
	defer stopResize()

	if err := session.Shell(); err != nil {
		return err
	}
	return session.Wait()
}

func normalizeLoginOptions(opts LoginOptions) LoginOptions {
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	return opts
}

func loginTerminalSize(fd int) (width int, height int) {
	width, height = defaultPTYWidth, defaultPTYHeight
	if w, h, err := term.GetSize(fd); err == nil && w > 0 && h > 0 {
		width, height = w, h
	}
	return width, height
}

func loginTermName(explicit string) string {
	name := strings.TrimSpace(explicit)
	if name == "" {
		name = strings.TrimSpace(os.Getenv("TERM"))
	}
	if name == "" {
		return defaultPTYTerm
	}
	return name
}

func executeRemoteScript(client RemoteClient, opts RunOptions) ([]byte, error) {
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

	if _, err := client.Run("chmod " + remoteScriptMode(opts) + " " + shellQuote(remoteScriptPath)); err != nil {
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

func uploadRemoteScript(client RemoteClient, localPath, remotePath string) error {
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
	return NewRemoteScriptPathInDir(value, "/tmp")
}

func NewRemoteScriptPathInDir(value time.Time, dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "/tmp"
	}
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return JoinRemotePath(dir, fmt.Sprintf("sshc-run-%d.sh", value.UnixNano()))
	}
	return JoinRemotePath(dir, fmt.Sprintf("sshc-run-%d-%x.sh", value.UnixNano(), suffix[:]))
}

func remoteScriptMode(opts RunOptions) string {
	if strings.TrimSpace(opts.SudoUser) != "" {
		return "644"
	}
	return "700"
}

func UploadRemote(host Host, localPath, remotePath string, opts TransferOptions) (result TransferResult, err error) {
	return UploadRemoteBatch(host, []TransferJob{{
		LocalPath:  localPath,
		RemotePath: remotePath,
		RemoteDir:  isLocalGlob(localPath),
	}}, opts)
}

func UploadRemoteBatch(host Host, jobs []TransferJob, opts TransferOptions) (result TransferResult, err error) {
	started := time.Now()
	defer func() {
		result.Elapsed = time.Since(started)
	}()

	expandedJobs, err := expandUploadJobs(jobs, opts)
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

	if opts.RemoveDir {
		if err := validateRemoteRemoveDirPath(expandedJobs[0].RemotePath); err != nil {
			return result, err
		}
		if _, err := client.Run("rm -rf -- " + shellQuote(expandedJobs[0].RemotePath)); err != nil {
			return result, err
		}
	}

	for _, job := range expandedJobs {
		partial, err := uploadPreparedJob(client, sftpClient, job, opts)
		if err != nil {
			return result, err
		}
		result.add(partial)
	}
	if opts.SHA256 && result.Files > 0 {
		result.SHA256OK = true
	}
	return result, nil
}

func expandUploadJobs(jobs []TransferJob, opts TransferOptions) ([]TransferJob, error) {
	if len(jobs) == 0 {
		return nil, fmt.Errorf("upload job is required")
	}
	if opts.RemoveDir && len(jobs) != 1 {
		return nil, fmt.Errorf("--remove-dir is only supported for a single directory upload")
	}

	expanded := make([]TransferJob, 0, len(jobs))
	for _, job := range jobs {
		localPath := strings.TrimSpace(job.LocalPath)
		remotePath := strings.TrimSpace(job.RemotePath)
		if localPath == "" {
			return nil, fmt.Errorf("local path is required")
		}
		if remotePath == "" {
			return nil, fmt.Errorf("remote path is required")
		}
		if isLocalGlob(localPath) {
			if opts.RemoveDir {
				return nil, fmt.Errorf("--remove-dir is only supported for directory uploads")
			}
			matches, err := expandLocalGlob(localPath)
			if err != nil {
				return nil, err
			}
			for _, localFile := range matches {
				expanded = append(expanded, TransferJob{LocalPath: localFile, RemotePath: JoinRemotePath(remotePath, filepath.Base(localFile))})
			}
			continue
		}

		info, err := os.Stat(localPath)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() && opts.RemoveDir {
			return nil, fmt.Errorf("--remove-dir is only supported for directory uploads")
		}
		if info.IsDir() && opts.SHA256 {
			return nil, fmt.Errorf("--sha256 is only supported for file transfers")
		}
		if job.RemoteDir {
			remotePath = JoinRemotePath(remotePath, filepath.Base(localPath))
		} else if !info.IsDir() {
			remotePath = RemoteFilePath(localPath, remotePath)
		}
		expanded = append(expanded, TransferJob{LocalPath: localPath, RemotePath: remotePath})
	}
	return expanded, nil
}

func uploadPreparedJob(client RemoteClient, sftpClient *sftp.Client, job TransferJob, opts TransferOptions) (result TransferResult, err error) {
	info, err := os.Stat(job.LocalPath)
	if err != nil {
		return result, err
	}
	if !info.IsDir() {
		if err := mkdirRemoteParent(sftpClient, job.RemotePath); err != nil {
			return result, err
		}
		if opts.SHA256 {
			result.LocalSHA256, err = fileSHA256(job.LocalPath)
			if err != nil {
				return result, err
			}
		}
		bytes, err := uploadFileWithSFTP(sftpClient, job.LocalPath, job.RemotePath)
		if err != nil {
			return result, err
		}
		result.Bytes += bytes
		result.Files++
		if opts.SHA256 {
			result.RemoteSHA256, err = remoteSHA256(client, job.RemotePath)
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

	root := filepath.Clean(job.LocalPath)
	err = filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}

		remoteCurrent := job.RemotePath
		if rel != "." {
			remoteCurrent = JoinRemotePath(job.RemotePath, filepath.ToSlash(rel))
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

func (result *TransferResult) add(partial TransferResult) {
	result.Bytes += partial.Bytes
	result.Files += partial.Files
	result.Directories += partial.Directories
	if partial.LocalSHA256 != "" {
		result.LocalSHA256 = partial.LocalSHA256
	}
	if partial.RemoteSHA256 != "" {
		result.RemoteSHA256 = partial.RemoteSHA256
	}
	if partial.SHA256OK {
		result.SHA256OK = true
	}
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

func newSSHClient(host Host) (RemoteClient, error) {
	if strings.TrimSpace(host.Jump) == "" {
		return newDirectSSHClient(host)
	}
	conn, err := ResolveConnectionForHost(host)
	if err != nil {
		return nil, err
	}
	return newSSHClientForConnection(conn)
}

func newSSHClientForConnection(conn ResolvedConnection) (RemoteClient, error) {
	if conn.Jump == nil {
		return newDirectSSHClient(conn.Target)
	}

	jump, err := newDirectSSHClient(*conn.Jump)
	if err != nil {
		return nil, fmt.Errorf("connect jump host %s: %w", HostLogName(*conn.Jump), err)
	}

	targetAddr := net.JoinHostPort(conn.Target.IP, fmt.Sprint(conn.Target.Port))
	rawConn, err := jump.Dial("tcp", targetAddr)
	if err != nil {
		_ = jump.Close()
		return nil, fmt.Errorf("connect target host %s via jump %s: %w", HostLogName(conn.Target), HostLogName(*conn.Jump), err)
	}

	config, err := sshClientConfig(conn.Target)
	if err != nil {
		_ = rawConn.Close()
		_ = jump.Close()
		return nil, err
	}
	sshConn, chans, reqs, err := newSSHClientConn(rawConn, targetAddr, config)
	if err != nil {
		_ = rawConn.Close()
		_ = jump.Close()
		return nil, fmt.Errorf("connect target host %s via jump %s: %w", HostLogName(conn.Target), HostLogName(*conn.Jump), err)
	}

	target := &goph.Client{
		Client: ssh.NewClient(sshConn, chans, reqs),
		Config: gophConfig(conn.Target, goph.Auth(config.Auth), config.Timeout, config.HostKeyCallback),
	}
	return &remoteClient{
		Client: target,
		closeAll: func() error {
			var targetErr error
			if target.Client != nil {
				targetErr = target.Close()
			}
			jumpErr := jump.Close()
			if targetErr != nil {
				return targetErr
			}
			return jumpErr
		},
		dial: target.Dial,
	}, nil
}

func newDirectSSHClient(host Host) (*remoteClient, error) {
	config, err := gophClientConfig(host)
	if err != nil {
		return nil, err
	}
	client, err := newGophConn(config)
	if err != nil {
		return nil, err
	}
	return &remoteClient{
		Client: client,
		closeAll: func() error {
			if client.Client == nil {
				return nil
			}
			return client.Close()
		},
		dial: client.Dial,
	}, nil
}

func gophClientConfig(host Host) (*goph.Config, error) {
	auth, err := hostAuth(host)
	if err != nil {
		return nil, err
	}
	timeout, err := clientConnectTimeout(host)
	if err != nil {
		return nil, err
	}
	callback, err := hostKeyCallback(host)
	if err != nil {
		return nil, err
	}
	return gophConfig(host, auth, timeout, callback), nil
}

func gophConfig(host Host, auth goph.Auth, timeout time.Duration, callback ssh.HostKeyCallback) *goph.Config {
	return &goph.Config{
		User:     host.User,
		Addr:     host.IP,
		Port:     uint(host.Port),
		Auth:     auth,
		Timeout:  timeout,
		Callback: callback,
	}
}

func sshClientConfig(host Host) (*ssh.ClientConfig, error) {
	gophConfig, err := gophClientConfig(host)
	if err != nil {
		return nil, err
	}
	return &ssh.ClientConfig{
		User:            gophConfig.User,
		Auth:            gophConfig.Auth,
		Timeout:         gophConfig.Timeout,
		HostKeyCallback: gophConfig.Callback,
		BannerCallback:  gophConfig.BannerCallback,
	}, nil
}

func clientConnectTimeout(host Host) (time.Duration, error) {
	timeout, err := ParseTimeout(host.ConnectTimeout)
	if err != nil {
		return 0, err
	}
	if timeout <= 0 {
		return 20 * time.Second, nil
	}
	return timeout, nil
}

func hostKeyCallback(host Host) (ssh.HostKeyCallback, error) {
	switch strings.TrimSpace(host.HostKeyCheck) {
	case "", HostKeyCheckKnownHosts:
		path := strings.TrimSpace(host.KnownHostsPath)
		if path == "" {
			path = DefaultKnownHostsPath
		}
		path = expandUserPath(path)
		callback, err := knownhosts.New(path)
		if err != nil {
			return nil, fmt.Errorf("load known_hosts %s: %w", path, err)
		}
		return callback, nil
	case HostKeyCheckInsecure:
		return ssh.InsecureIgnoreHostKey(), nil
	default:
		return nil, fmt.Errorf("invalid host_key_check %q, want known_hosts or insecure", host.HostKeyCheck)
	}
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

func remoteSHA256(client RemoteClient, remotePath string) (string, error) {
	command := "command -v sha256sum >/dev/null 2>&1 || { echo 'sshc: remote sha256sum command not found' >&2; exit 127; }; sha256sum " + shellQuote(remotePath)
	out, err := client.Run(command)
	if err != nil {
		return "", err
	}
	return parseSHA256SumOutput(string(out))
}
