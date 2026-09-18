package store

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

var reopenCredentialFile = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReOpenFile")

func secureCredentialFile(f *os.File, directory bool) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("credential storage must not use reparse points")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
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
	// 重新打开同一对象，避免再次解析路径；Go 的普通文件句柄
	// 不包含修改权限所需的 WRITE_DAC。
	if err = reopenCredentialFile.Find(); err != nil {
		return err
	}
	handle, _, callErr := reopenCredentialFile.Call(
		f.Fd(),
		uintptr(windows.READ_CONTROL|windows.WRITE_DAC),
		uintptr(windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE),
		uintptr(windows.FILE_FLAG_BACKUP_SEMANTICS),
	)
	if windows.Handle(handle) == windows.InvalidHandle {
		if callErr != windows.ERROR_SUCCESS {
			return callErr
		}
		return errors.New("could not secure credential storage")
	}
	defer windows.CloseHandle(windows.Handle(handle))
	if err = windows.SetSecurityInfo(windows.Handle(handle), windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		return err
	}
	applied, err := windows.GetSecurityInfo(windows.Handle(handle), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
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
