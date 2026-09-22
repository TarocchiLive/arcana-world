package tui

import "golang.org/x/sys/windows"

func preferQRImage() bool {
	version := windows.RtlGetVersion()
	// Windows 11 的主版本号仍为 10，内部版本号从 22000 起。
	return version.MajorVersion < 10 || (version.MajorVersion == 10 && version.BuildNumber < 22000)
}
