//go:build darwin || linux

package rtmpsrt

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ebitengine/purego"
)

func openSRT() (func(string) (uintptr, error), error) {
	names := []string{"libsrt.so.1.5", "libsrt.so", "libsrt-gnutls.so.1.5", "libsrt-openssl.so.1.5"}
	if runtime.GOOS == "darwin" {
		names = []string{"libsrt.1.5.dylib", "libsrt.dylib"}
	}
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		for _, name := range names {
			candidates = append(candidates, filepath.Join(filepath.Dir(exe), "lib", name))
		}
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/opt/homebrew/opt/srt/lib/libsrt.dylib")
	}
	candidates = append(candidates, names...)
	for _, name := range candidates {
		h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err == nil {
			return func(symbol string) (uintptr, error) { return purego.Dlsym(h, symbol) }, nil
		}
	}
	return nil, errors.New("RTMPSRT requires the optional libsrt runtime library")
}
