package tui

import "golang.org/x/sys/windows"

func preferQRImage() bool {
	version := windows.RtlGetVersion()
	// Windows 11 retains major version 10 and starts at build 22000.
	return version.MajorVersion < 10 || (version.MajorVersion == 10 && version.BuildNumber < 22000)
}
