//go:build !windows

package journal

import "os"

func openLogFile(path string, flags int) (*os.File, error) {
	return os.OpenFile(path, flags, 0600)
}
