package command

import "sshc/internal/core"

func setRunRemoteForTest(fn func(core.Host, string, core.RunOptions) ([]byte, error)) func() {
	old := runRemote
	runRemote = fn
	return func() { runRemote = old }
}

func setUploadRemoteForTest(fn func(core.Host, string, string) error) func() {
	old := scpUpload
	scpUpload = fn
	return func() { scpUpload = old }
}

func setDownloadRemoteForTest(fn func(core.Host, string, string) error) func() {
	old := downloadRemote
	downloadRemote = fn
	return func() { downloadRemote = old }
}
