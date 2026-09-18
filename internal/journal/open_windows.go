package journal

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 删除共享允许清理过期记录时按 POSIX 重命名语义替换已打开的日志。
// 独立的日志锁文件负责排除其他写入者。
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

func replaceLogFile(next *os.File, path string) error {
	fail := func(err error) error {
		return &os.LinkError{Op: "rename", Old: next.Name(), New: path, Err: err}
	}
	source, err := windows.UTF16PtrFromString(next.Name())
	if err != nil {
		return fail(err)
	}
	target, err := windows.UTF16FromString(path)
	if err != nil {
		return fail(err)
	}
	handle, err := windows.CreateFile(source, windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return fail(err)
	}
	defer windows.CloseHandle(handle)

	// 即使启用了删除共享，MoveFileEx（os.Rename）也无法替换仍有打开句柄的目标。
	// 使用 POSIX 替换语义，在提交成功前保持新旧两个追加句柄有效。
	type fileRenameInfo struct {
		Flags          uint32
		RootDirectory  windows.Handle
		FileNameLength uint32
		FileName       [1]uint16
	}
	var info fileRenameInfo
	size := int(unsafe.Offsetof(info.FileName)) + len(target)*2
	buffer := make([]byte, size)
	rename := (*fileRenameInfo)(unsafe.Pointer(&buffer[0]))
	rename.Flags = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	rename.FileNameLength = uint32((len(target) - 1) * 2)
	copy(unsafe.Slice(&rename.FileName[0], len(target)), target)
	if err := windows.SetFileInformationByHandle(handle, windows.FileRenameInfoEx, &buffer[0], uint32(size)); err != nil {
		return fail(err)
	}
	return nil
}
