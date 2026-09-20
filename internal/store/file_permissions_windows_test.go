package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/zalando/go-keyring"
	"golang.org/x/sys/windows"
)

func TestWindowsFileBackendProtectedPermissions(t *testing.T) {
	dir := t.TempDir()
	backend := newFileBackend(dir)
	for _, value := range []string{"initial-secret", "updated-secret"} {
		if err := backend.Set("user", value); err != nil {
			t.Fatalf("file backend Set: %v", err)
		}
		got, err := backend.Get("user")
		if err != nil {
			t.Fatalf("file backend Get: %v", err)
		}
		if got != value {
			t.Fatal("file backend did not preserve the latest secret")
		}
		assertWindowsCredentialDACL(t, filepath.Join(dir, "credentials"), true)
		assertWindowsCredentialDACL(t, filepath.Join(dir, "credentials", credentialFile), false)
	}
	if err := backend.Delete("user"); err != nil {
		t.Fatalf("file backend Delete: %v", err)
	}
	if _, err := backend.Get("user"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("deleted credential lookup: %v", err)
	}
}

func TestWindowsCredentialPermissionsFollowOpenObject(t *testing.T) {
	for _, directory := range []bool{true, false} {
		name := "file"
		if directory {
			name = "directory"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if directory {
				if err := root.Mkdir("original", 0700); err != nil {
					t.Fatal(err)
				}
			}
			flags := os.O_RDONLY
			if !directory {
				flags = os.O_CREATE | os.O_EXCL | os.O_RDWR
			}
			f, err := root.OpenFile("original", flags, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if err := root.Rename("original", "moved"); err != nil {
				t.Fatal(err)
			}
			// 原路径已不存在，权限更新仍必须作用于已打开的对象。
			if err := secureCredentialFile(f, directory); err != nil {
				t.Fatalf("secure renamed %s: %v", name, err)
			}
			assertWindowsCredentialDACL(t, filepath.Join(dir, "moved"), directory)
		})
	}
}

func assertWindowsCredentialDACL(t *testing.T, path string, directory bool) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	applied, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("read persisted credential DACL: %v", err)
	}
	control, _, err := applied.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("credential DACL allows inherited permissions")
	}
	acl, _, err := applied.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if acl == nil || acl.AceCount != 2 {
		t.Fatal("credential DACL must grant access only to the current user and SYSTEM")
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatal(err)
	}
	flags := uint8(0)
	if directory {
		flags = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	seenUser, seenSystem := false, false
	for index := uint32(0); index < uint32(acl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, index, &ace); err != nil {
			t.Fatal(err)
		}
		// FILE_ALL_ACCESS：标准权限、同步权限和全部文件专用权限。
		const fileAllAccess = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != flags || ace.Mask != fileAllAccess {
			t.Fatal("credential DACL has unexpected permissions or inheritance")
		}
		// 比较 SID 本身，避免 LA 等别名影响权限判断。
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		switch {
		case sid.Equals(user.User.Sid):
			seenUser = true
		case sid.Equals(system):
			seenSystem = true
		default:
			t.Fatal("credential DACL grants access to an unexpected principal")
		}
	}
	if !seenUser || !seenSystem {
		t.Fatal("credential DACL is missing the current user or SYSTEM")
	}
}
