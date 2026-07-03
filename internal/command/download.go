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

		if err := downloadRemote(host, remotePath, localPath); err != nil {
			return err
		}
		fmt.Fprintf(c.Output(), "downloaded %s:%s to %s\n", core.HostLogName(host), remotePath, localPath)
		return nil
	})
	cmd.Aliases = []string{"dl"}
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc download -r /tmp/remote-file.txt -l ./local-file.txt devhost
  sshc download -r /tmp/remote-file.txt -l ./downloads/ devhost
  sshc dl -r /tmp/remote-dir -l ./local-dir devhost

Path rules:
  - -r/--remote can be a remote file or directory.
  - -l/--local is the local destination path.
  - If local path exists as a directory, the remote base name is appended.
  - If local path ends with / or \, the remote base name is appended.
  - Directory download recursively creates local directories and files.
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&downloadOpts.LocalPath, "local", "", "local destination path;true;l")
		c.StringVar(&downloadOpts.RemotePath, "remote", "", "remote file or directory path;true;r")
		c.AddArg("target", "host ip or name", true)
	}
	return cmd
}
