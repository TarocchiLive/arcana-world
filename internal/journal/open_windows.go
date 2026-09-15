package journal

import (
	"os"

	"golang.org/x/sys/windows"
)

// Delete sharing lets retention atomically replace the pathname while this
// process holds the old append handle. The stable journal lock excludes writers.
func openLogFile(path string, flags int) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	creation := uint32(windows.OPEN_EXISTING)
	if flags&os.O_CREATE != 0 {
		creation = windows.CREATE_NEW
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.FILE_APPEND_DATA|windows.FILE_WRITE_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, creation, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}
