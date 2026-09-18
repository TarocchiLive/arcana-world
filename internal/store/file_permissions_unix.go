//go:build !windows

package store

import "os"

func secureCredentialFile(f *os.File, directory bool) error {
	mode := os.FileMode(0600)
	if directory {
		mode = 0700
	}
	return f.Chmod(mode)
}
