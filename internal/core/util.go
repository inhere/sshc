package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func withRemoteCWD(command, cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return command
	}
	return "cd " + shellQuote(cwd) + " && " + command
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func remoteDuration(value time.Duration) string {
	return fmt.Sprintf("%gs", value.Seconds())
}

func scriptExecuteCommand(remotePath string) string {
	return "bash " + shellQuote(remotePath)
}

func effectiveKillAfter(value time.Duration) time.Duration {
	if value <= 0 {
		return defaultRemoteKillAfter
	}
	return value
}

func expandUserPath(filePath string) string {
	filePath = strings.TrimSpace(filePath)
	if filePath == "~" {
		home, err := userHomeDir()
		if err == nil {
			return home
		}
		return filePath
	}
	if strings.HasPrefix(filePath, "~/") || strings.HasPrefix(filePath, `~\`) {
		home, err := userHomeDir()
		if err == nil {
			return filepath.Join(home, filePath[2:])
		}
	}
	return filePath
}

func RemoteFilePath(localPath, remotePath string) string {
	if strings.HasSuffix(remotePath, "/") {
		return JoinRemotePath(remotePath, filepath.Base(localPath))
	}
	return remotePath
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

func isLocalGlob(localPath string) bool {
	return strings.ContainsAny(localPath, "*?[")
}

func expandLocalGlob(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("local glob %q matched no files", pattern)
	}
	sort.Strings(matches)
	files := make([]string, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			return nil, fmt.Errorf("local glob %q matched directory %q; only files are supported", pattern, match)
		}
		files = append(files, match)
	}
	return files, nil
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

func validateRemoteRemoveDirPath(remotePath string) error {
	remotePath = strings.TrimSpace(remotePath)
	cleaned := path.Clean(remotePath)
	if remotePath == "" || cleaned == "." || cleaned == "/" {
		return fmt.Errorf("--remove-dir requires a non-root remote directory")
	}
	return nil
}
