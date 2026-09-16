//go:build !windows

package overlay

import (
	"fmt"
	"os"
	"os/exec"
)

func privateSocketDir(root string) (string, error) {
	// 避免 macOS 默认临时路径过长；子目录由操作系统原子创建为 0700。
	if root == "" {
		root = "/tmp"
	}
	directory, err := os.MkdirTemp(root, "ao-")
	if err != nil {
		return "", fmt.Errorf("overlay: creating private runtime directory in %q: %w", root, err)
	}
	if err = os.Chmod(directory, 0700); err != nil {
		_ = os.RemoveAll(directory)
		return "", err
	}
	return directory, nil
}
func secureSocket(path string) error { return os.Chmod(path, 0600) }
func setupChild(cmd *exec.Cmd)       {}
