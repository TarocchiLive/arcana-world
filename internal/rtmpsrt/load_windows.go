package rtmpsrt

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

func openSRT() (func(string) (uintptr, error), error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// 仅加载发行包私有目录，禁止从当前工作目录搜索 DLL。
	path := filepath.Join(filepath.Dir(exe), "lib", "srt.dll")
	h, err := windows.LoadLibraryEx(path, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_DEFAULT_DIRS)
	if err != nil {
		return nil, errors.New("RTMPSRT requires the optional libsrt runtime library")
	}
	return func(symbol string) (uintptr, error) { return windows.GetProcAddress(h, symbol) }, nil
}
