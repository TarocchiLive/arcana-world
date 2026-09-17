// Package helperpath locates helpers within the application's installation.
package helperpath

import (
	"path/filepath"
	"runtime"
)

// Installed follows the application executable's symlinks and locates name in
// its libexec directory. Callers retain responsibility for checking the helper.
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
