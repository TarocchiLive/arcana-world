// helperpath 包定位应用安装目录中的辅助程序。
package helperpath

import (
	"path/filepath"
	"runtime"
)

// Installed 解析应用可执行文件的符号链接，并在其 libexec 目录中定位 name。
// 调用方仍负责检查该辅助程序。
func Installed(executable, name string) (string, error) {
	executable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(executable), "libexec", name), nil
}
