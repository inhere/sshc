package command

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"sshc/internal/core"

	"github.com/gookit/goutil/cflag/capp"
)

var (
	scpOpts = struct {
		LocalPath  string
		RemotePath string
		SHA256     bool
		RemoveDir  bool
	}{}

	scpUpload = core.UploadRemote
)

func NewUploadCmd() *capp.Cmd {
	cmd := capp.NewCmd("scp", "upload a file or directory to remote host", func(c *capp.Cmd) error {
		target := strings.TrimSpace(c.Arg("target").String())
		localPath := strings.TrimSpace(scpOpts.LocalPath)
		remotePath := strings.TrimSpace(scpOpts.RemotePath)
		if localPath == "" {
			return errors.New("local path is required")
		}
		if remotePath == "" {
			return errors.New("remote path is required")
		}

		store, err := core.LoadStore()
		if err != nil {
			return err
		}
		host, ok, err := store.ResolveHost(target)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("host %q not found", target)
		}

		result, err := scpUpload(host, localPath, remotePath, core.TransferOptions{
			SHA256:    scpOpts.SHA256,
			RemoveDir: scpOpts.RemoveDir,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(c.Output(), "uploaded %s to %s:%s\n", localPath, core.HostLogName(host), remotePath)
		fmt.Fprintf(c.Output(), "size=%d files=%d dirs=%d elapsed=%s\n", result.Bytes, result.Files, result.Directories, formatElapsed(result.Elapsed))
		writeSHA256Result(c, result)
		return nil
	})
	cmd.Aliases = []string{"upload"}
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost
  sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost --sha256
  sshc scp -l ./local-dir -r /tmp/remote-dir devhost
  sshc scp -l ./dist -r /opt/app/dist devhost --remove-dir
  sshc scp -l "./dist/*.jar" -r /opt/app/lib devhost
  sshc upload -l ./dist -r /opt/app/dist devhost

Path rules:
  - -l/--local can be a file or directory.
  - -r/--remote is the remote destination path.
  - File upload creates remote parent directories when needed.
  - If remote path ends with / for file upload, the local file name is appended.
  - Directory upload recursively creates directories and files under the remote path.
  - Local glob patterns are expanded by sshc and upload matching files to the remote directory.
  - --sha256 verifies file uploads with local and remote sha256 hashes.
  - --sha256 is only supported for single file uploads.
  - --remove-dir removes the remote directory before uploading a local directory.
  - --remove-dir refuses empty, current, and root remote paths.
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&scpOpts.LocalPath, "local", "", "local file or directory path;true;l")
		c.StringVar(&scpOpts.RemotePath, "remote", "", "remote file or directory path;true;r")
		c.BoolVar(&scpOpts.SHA256, "sha256", false, "verify file transfer with sha256")
		c.BoolVar(&scpOpts.RemoveDir, "remove-dir", false, "remove remote directory before directory upload")
		c.AddArg("target", "host ip or name", true)
	}
	return cmd
}

func formatElapsed(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	return value.Round(time.Millisecond).String()
}

func writeSHA256Result(c *capp.Cmd, result core.TransferResult) {
	if result.LocalSHA256 == "" && result.RemoteSHA256 == "" {
		return
	}
	fmt.Fprintf(c.Output(), "sha256.local=%s\n", result.LocalSHA256)
	fmt.Fprintf(c.Output(), "sha256.remote=%s\n", result.RemoteSHA256)
	fmt.Fprintf(c.Output(), "sha256.ok=%v\n", result.SHA256OK)
}
