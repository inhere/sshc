package command

import (
	"errors"
	"fmt"
	"strings"

	"sshc/internal/core"

	"github.com/gookit/goutil/cflag/capp"
)

var (
	downloadOpts = struct {
		LocalPath  string
		RemotePath string
		SHA256     bool
	}{}

	downloadRemote = core.FetchRemote
)

func NewDownloadCmd() *capp.Cmd {
	cmd := capp.NewCmd("download", "download a file or directory from remote host", func(c *capp.Cmd) error {
		target := strings.TrimSpace(c.Arg("target").String())
		localPath := strings.TrimSpace(downloadOpts.LocalPath)
		remotePath := strings.TrimSpace(downloadOpts.RemotePath)
		if localPath == "" {
			return errors.New("local path is required")
		}
		if remotePath == "" {
			return errors.New("remote path is required")
		}

		store, err := core.LoadStoreWithSSHConfig()
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

		result, err := downloadRemote(host, remotePath, localPath, core.TransferOptions{SHA256: downloadOpts.SHA256})
		if err != nil {
			return err
		}
		fmt.Fprintf(c.Output(), "downloaded %s:%s to %s\n", core.HostLogName(host), remotePath, localPath)
		fmt.Fprintf(c.Output(), "size=%d files=%d dirs=%d elapsed=%s\n", result.Bytes, result.Files, result.Directories, formatElapsed(result.Elapsed))
		writeSHA256Result(c, result)
		return nil
	})
	cmd.Aliases = []string{"dl"}
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc download -r /tmp/remote-file.txt -l ./local-file.txt devhost
  sshc download -r /tmp/remote-file.txt -l ./local-file.txt devhost --sha256
  sshc download -r /tmp/remote-file.txt -l ./downloads/ devhost
  sshc dl -r /tmp/remote-dir -l ./local-dir devhost

Path rules:
  - Existing local directories receive the remote base name.
  - Local paths ending with / or \ also receive the remote base name.
  - Directory download recursively creates local directories and files.
  - SHA256 verification is available for single file downloads only.
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&downloadOpts.LocalPath, "local", "", "local destination path;true;l")
		c.StringVar(&downloadOpts.RemotePath, "remote", "", "remote file or directory path;true;r")
		c.BoolVar(&downloadOpts.SHA256, "sha256", false, "verify file transfer with sha256")
		c.AddArg("target", "host ip or name", true)
	}
	return cmd
}
