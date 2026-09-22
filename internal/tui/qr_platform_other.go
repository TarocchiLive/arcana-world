//go:build !windows

package tui

func preferQRImage() bool {
	return false
}
