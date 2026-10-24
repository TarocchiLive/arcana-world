// helperpath 包定位应用安装目录中的辅助程序。
package helperpath

import (
	"os"
	"path/filepath"
	"runtime"
)

// ApplicationDir 返回当前应用可执行文件解析符号链接后所在的目录。
func ApplicationDir() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return executableDir(executable)
}

// Installed 解析应用可执行文件的符号链接，并在其 libexec 目录中定位 name。
// 调用方仍负责检查该辅助程序。
func Installed(executable, name string) (string, error) {
	dir, err := executableDir(executable)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, "libexec", name), nil
}

func executableDir(executable string) (string, error) {
	executable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	return filepath.Dir(executable), nil
}
