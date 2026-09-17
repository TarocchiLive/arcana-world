//go:build linux && cgo && wayland

package native

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWaylandLayoutKeepsWrappedTail(t *testing.T) {
	flags, err := exec.Command("pkg-config", "--cflags", "--libs", "wayland-client", "pangocairo").CombinedOutput()
	if err != nil {
		t.Fatalf("native layout dependencies: %v\n%s", err, flags)
	}
	compiler := strings.Fields(os.Getenv("CC"))
	if len(compiler) == 0 {
		compiler = []string{"cc"}
	}
	binary := filepath.Join(t.TempDir(), "wayland-layout")
	args := append(compiler[1:], "-std=c11", "-UNDEBUG", "-o", binary, "testdata/wayland_layout.c")
	args = append(args, strings.Fields(string(flags))...)
	args = append(args, "-lm", "-lpthread")
	if output, err := exec.Command(compiler[0], args...).CombinedOutput(); err != nil {
		t.Fatalf("compile native layout regression: %v\n%s", err, output)
	}
	if output, err := exec.Command(binary).CombinedOutput(); err != nil {
		t.Fatalf("native layout regression: %v\n%s", err, output)
	}
}
