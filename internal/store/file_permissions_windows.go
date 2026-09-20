package store

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func secureCredentialFile(f *os.File, directory bool) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return fmt.Errorf("inspect credential storage handle: %w", err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("credential storage must not use reparse points")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("get credential storage user: %w", err)
	}
	inheritance := ""
	if directory {
		inheritance = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;" + inheritance + ";FA;;;" + user.User.Sid.String() + ")(A;" + inheritance + ";FA;;;SY)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	// os.Root 使用 NtCreateFile 打开目录；ReOpenFile 的非目录打开语义
	// 不能用于这些句柄。以原句柄和空名称重新打开同一对象，不再解析路径。
	// 普通文件句柄不含 WRITE_DAC，因此文件也使用同样的方式申请 ACL 权限。
	name := windows.NTUnicodeString{}
	attrs := windows.OBJECT_ATTRIBUTES{
		RootDirectory: windows.Handle(f.Fd()),
		ObjectName:    &name,
	}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	options := uint32(windows.FILE_NON_DIRECTORY_FILE)
	if directory {
		options = windows.FILE_DIRECTORY_FILE
	}
	var handle windows.Handle
	if err = windows.NtCreateFile(&handle, windows.READ_CONTROL|windows.WRITE_DAC|windows.SYNCHRONIZE,
		&attrs, &windows.IO_STATUS_BLOCK{}, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN, options|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT,
		0, 0); err != nil {
		return fmt.Errorf("reopen credential storage ACL handle (directory=%t): %w", directory, err)
	}
	defer windows.CloseHandle(handle)
	if err = windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("set credential storage DACL: %w", err)
	}
	applied, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read credential storage DACL: %w", err)
	}
	control, _, err := applied.Control()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("credential storage does not support protected permissions")
	}
	// 自动继承标志不影响实际授权，比较前忽略这些标志。
	if err = applied.SetControl(windows.SE_DACL_AUTO_INHERITED|windows.SE_DACL_AUTO_INHERIT_REQ, 0); err != nil {
		return err
	}
	if applied.String() != sd.String() {
		return errors.New("credential storage permissions could not be verified")
	}
	return nil
}
