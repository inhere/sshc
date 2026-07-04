package command

import "github.com/inhere/sshc/internal/core"

func setRunRemoteForTest(fn func(core.Host, string, core.RunOptions) ([]byte, error)) func() {
	old := runRemote
	runRemote = fn
	return func() { runRemote = old }
}

func setUploadRemoteForTest(fn func(core.Host, []core.TransferJob, core.TransferOptions) (core.TransferResult, error)) func() {
	old := scpUpload
	scpUpload = fn
	return func() { scpUpload = old }
}

func setDownloadRemoteForTest(fn func(core.Host, string, string, core.TransferOptions) (core.TransferResult, error)) func() {
	old := downloadRemote
	downloadRemote = fn
	return func() { downloadRemote = old }
}

func setReadClipboardForTest(fn func() (string, error)) func() {
	old := readClipboard
	readClipboard = fn
	return func() { readClipboard = old }
}

func setReadInteractivePasswordForTest(fn func(...string) string) func() {
	old := readInteractivePassword
	readInteractivePassword = fn
	return func() { readInteractivePassword = old }
}

func setLoginRemoteForTest(fn func(core.Host, core.LoginOptions) error) func() {
	old := loginRemote
	loginRemote = fn
	return func() { loginRemote = old }
}
