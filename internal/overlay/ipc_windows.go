//go:build windows

package overlay

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func privateSocketDir(root string) (string, error) {
	if root == "" {
		root = os.TempDir()
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	// 在创建时设置受保护且可继承的 DACL，不留下先创建再收紧权限的窗口。
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return "", err
	}
	attributes := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	var random [8]byte
	if _, err = rand.Read(random[:]); err != nil {
		return "", err
	}
	directory := filepath.Join(root, "ao-"+hex.EncodeToString(random[:]))
	path, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return "", err
	}
	if err = windows.CreateDirectory(path, &attributes); err != nil {
		return "", fmt.Errorf("overlay: creating private runtime directory in %q: %w", root, err)
	}
	return directory, nil
}

// 套接字继承私有目录的用户/SYSTEM ACL；Windows 的 chmod 不能设置此权限。
func secureSocket(path string) error { return nil }
func setupChild(cmd *exec.Cmd)       { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
