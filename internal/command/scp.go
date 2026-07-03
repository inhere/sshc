package command

import (
	"errors"
	"fmt"
	"strings"

	"sshc/internal/core"

	"github.com/gookit/goutil/cflag/capp"
)

var (
	scpOpts = struct {
		LocalPath  string
		RemotePath string
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

		if err := scpUpload(host, localPath, remotePath); err != nil {
			return err
		}
		fmt.Fprintf(c.Output(), "uploaded %s to %s:%s\n", localPath, core.HostLogName(host), remotePath)
		return nil
	})
	cmd.Aliases = []string{"upload"}
	cmd.LongHelp = strings.TrimSpace(`
Examples:
  sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost
  sshc scp -l ./local-dir -r /tmp/remote-dir devhost
  sshc upload -l ./dist -r /opt/app/dist devhost

Path rules:
  - -l/--local can be a file or directory.
  - -r/--remote is the remote destination path.
  - File upload creates remote parent directories when needed.
  - If remote path ends with / for file upload, the local file name is appended.
  - Directory upload recursively creates directories and files under the remote path.
`)
	cmd.OnAdd = func(c *capp.Cmd) {
		c.StringVar(&scpOpts.LocalPath, "local", "", "local file or directory path;true;l")
		c.StringVar(&scpOpts.RemotePath, "remote", "", "remote file or directory path;true;r")
		c.AddArg("target", "host ip or name", true)
	}
	return cmd
}
