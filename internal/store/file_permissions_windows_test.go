package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
	"golang.org/x/sys/windows"
)

func TestWindowsFileBackendProtectedPermissions(t *testing.T) {
	dir := t.TempDir()
	backend := newFileBackend(dir)
	for _, value := range []string{"initial-secret", "updated-secret"} {
		if err := backend.Set("service", "user", value); err != nil {
			t.Fatalf("file backend Set: %v", err)
		}
		got, err := backend.Get("service", "user")
		if err != nil {
			t.Fatalf("file backend Get: %v", err)
		}
		if got != value {
			t.Fatal("file backend did not preserve the latest secret")
		}
		assertWindowsCredentialDACL(t, filepath.Join(dir, "credentials"), true)
		assertWindowsCredentialDACL(t, filepath.Join(dir, "credentials", credentialFile), false)
	}
	if err := backend.Delete("service", "user"); err != nil {
		t.Fatalf("file backend Delete: %v", err)
	}
	if _, err := backend.Get("service", "user"); !errors.Is(err, keyring.ErrNotFound) {
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
	inheritance := ""
	if directory {
		inheritance = "OICI"
	}
	want := "D:P(A;" + inheritance + ";FA;;;" + user.User.Sid.String() + ")(A;" + inheritance + ";FA;;;SY)"
	// 自动继承状态不改变授权；保护标志和全部 ACE 必须保持严格一致。
	if err := applied.SetControl(windows.SE_DACL_AUTO_INHERITED|windows.SE_DACL_AUTO_INHERIT_REQ, 0); err != nil {
		t.Fatal(err)
	}
	if got := applied.String(); got != want {
		t.Fatalf("persisted credential DACL = %q, want %q", got, want)
	}
}
