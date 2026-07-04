package command

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/inhere/sshc/internal/core"

	"github.com/gookit/gcli/v3"
)

var scpUpload = core.UploadRemoteBatch

func NewUploadCmd() *gcli.Command {
	opts := &uploadFlagOptions{}
	cmd := &gcli.Command{
		Name:    "scp",
		Desc:    "upload a file or directory to remote host",
		Aliases: []string{"upload"},
		Help: strings.TrimSpace(`
Examples:
  sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost
  sshc scp -l ./local-file.txt -r /tmp/remote-file.txt devhost --sha256
  sshc scp -l ./local-dir -r /tmp/remote-dir devhost
  sshc scp -l ./dist -r /opt/app/dist devhost --remove-dir
  sshc scp -l "./dist/*.jar" -r /opt/app/lib devhost
  sshc upload -l ./dist -r /opt/app/dist devhost
  sshc upload -l ./a.jar -l ./b.jar -r /opt/app/lib/ devhost
  sshc upload --map ./config/app.yml=/etc/app/app.yml --map ./scripts/deploy.sh=/opt/app/deploy.sh devhost

Path rules:
  - File upload creates remote parent directories when needed.
  - If remote path ends with / for file upload, the local file name is appended.
  - Repeated -l/--local uploads every local path to the same remote directory.
  - --map uploads each local path to its own remote path and cannot be mixed with -l/-r.
  - Directory upload recursively creates directories and files under the remote path.
  - Local glob patterns are expanded by sshc and upload matching files to the remote directory.
  - SHA256 verification is available for file uploads only; directories are not supported.
  - Removing a remote directory refuses empty, current, and root remote paths.
`),
		Config: func(c *gcli.Command) {
			c.VarOpt(&opts.LocalPaths, "local", "l", "local file or directory path, repeatable")
			c.StrOpt(&opts.RemotePath, "remote", "r", "", "remote file or directory path")
			c.VarOpt(&opts.Maps, "map", "", "upload mapping local=remote, repeatable")
			c.BoolOpt(&opts.SHA256, "sha256", "", false, "verify file transfer with sha256")
			c.BoolOpt(&opts.RemoveDir, "remove-dir", "", false, "remove remote directory before directory upload")
			c.AddArg("target", "host ip or name", true)
		},
		Func: func(c *gcli.Command, _ []string) error {
			target := strings.TrimSpace(c.Arg("target").String())
			jobs, err := buildUploadJobs(*opts)
			if err != nil {
				return err
			}

			host, err := resolveCommandHost(target)
			if err != nil {
				return err
			}

			result, err := scpUpload(host, jobs, core.TransferOptions{
				SHA256:    opts.SHA256,
				RemoveDir: opts.RemoveDir,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmdOutput(c), "uploaded %d path(s) to %s\n", len(jobs), core.HostLogName(host))
			fmt.Fprintf(cmdOutput(c), "size=%d files=%d dirs=%d elapsed=%s\n", result.Bytes, result.Files, result.Directories, formatElapsed(result.Elapsed))
			writeSHA256Result(c, result)
			return nil
		},
	}
	return cmd
}

type uploadFlagOptions struct {
	LocalPaths gcli.Strings
	RemotePath string
	Maps       gcli.Strings
	SHA256     bool
	RemoveDir  bool
}

func buildUploadJobs(opts uploadFlagOptions) ([]core.TransferJob, error) {
	locals := trimStrings(opts.LocalPaths.Strings())
	maps := trimStrings(opts.Maps.Strings())
	remotePath := strings.TrimSpace(opts.RemotePath)

	if len(maps) > 0 {
		if len(locals) > 0 || remotePath != "" {
			return nil, errors.New("--map cannot be used with --local or --remote")
		}
		if opts.RemoveDir {
			return nil, errors.New("--remove-dir cannot be used with --map")
		}
		return parseUploadMaps(maps)
	}

	if len(locals) == 0 {
		return nil, errors.New("local path is required")
	}
	if remotePath == "" {
		return nil, errors.New("remote path is required")
	}
	if opts.RemoveDir && len(locals) != 1 {
		return nil, errors.New("--remove-dir is only supported for a single directory upload")
	}

	jobs := make([]core.TransferJob, 0, len(locals))
	remoteDir := len(locals) > 1
	for _, localPath := range locals {
		jobs = append(jobs, core.TransferJob{
			LocalPath:  localPath,
			RemotePath: remotePath,
			RemoteDir:  remoteDir,
		})
	}
	return jobs, nil
}

func parseUploadMaps(values []string) ([]core.TransferJob, error) {
	jobs := make([]core.TransferJob, 0, len(values))
	for _, value := range values {
		localPath, remotePath, ok := strings.Cut(value, "=")
		localPath = strings.TrimSpace(localPath)
		remotePath = strings.TrimSpace(remotePath)
		if !ok || localPath == "" || remotePath == "" {
			return nil, fmt.Errorf("invalid --map %q, want local=remote", value)
		}
		if strings.ContainsAny(localPath, "*?[") {
			return nil, fmt.Errorf("--map does not support local glob %q", localPath)
		}
		jobs = append(jobs, core.TransferJob{LocalPath: localPath, RemotePath: remotePath})
	}
	return jobs, nil
}

func trimStrings(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			items = append(items, value)
		}
	}
	return items
}

func formatElapsed(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	return value.Round(time.Millisecond).String()
}

func writeSHA256Result(c *gcli.Command, result core.TransferResult) {
	if result.LocalSHA256 == "" && result.RemoteSHA256 == "" {
		if result.SHA256OK {
			fmt.Fprintf(cmdOutput(c), "sha256.ok=%v\n", result.SHA256OK)
		}
		return
	}
	if result.Files > 1 {
		fmt.Fprintf(cmdOutput(c), "sha256.ok=%v\n", result.SHA256OK)
		return
	}
	fmt.Fprintf(cmdOutput(c), "sha256.local=%s\n", result.LocalSHA256)
	fmt.Fprintf(cmdOutput(c), "sha256.remote=%s\n", result.RemoteSHA256)
	fmt.Fprintf(cmdOutput(c), "sha256.ok=%v\n", result.SHA256OK)
}
