package overlay

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBundledExecutableFollowsMovedInstallationAndSymlink(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "原始 安装")
	if err := os.MkdirAll(filepath.Join(install, "libexec"), 0700); err != nil {
		t.Fatal(err)
	}
	name := "arcana-world-overlay"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, path := range []string{filepath.Join(install, "arcana-world"), filepath.Join(install, "libexec", name)} {
		if err := os.WriteFile(path, []byte("fixture"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	moved := filepath.Join(root, "移动 后")
	if err := os.Rename(install, moved); err != nil {
		t.Fatal(err)
	}
	// A helper in the working directory must not override the installed component.
	t.Chdir(root)
	if err := os.WriteFile(name, []byte("wrong helper"), 0700); err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T, entry string) {
		t.Helper()
		got, err := bundledExecutable(entry)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := os.Stat(got)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := os.Stat(filepath.Join(moved, "libexec", name))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(actual, expected) {
			t.Fatalf("resolved a helper outside the installation: %s", got)
		}
	}
	check(t, filepath.Join(moved, "arcana-world"))
	t.Run("launcher symlink", func(t *testing.T) {
		launcher := filepath.Join(root, "launcher")
		if err := os.Symlink(filepath.Join(moved, "arcana-world"), launcher); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlink privileges unavailable: %v", err)
			}
			t.Fatal(err)
		}
		check(t, launcher)
	})
	if err := os.Remove(filepath.Join(moved, "libexec", name)); err != nil {
		t.Fatal(err)
	}
	if _, err := bundledExecutable(filepath.Join(moved, "arcana-world")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing bundled helper fell back to another executable: %v", err)
	}
}
