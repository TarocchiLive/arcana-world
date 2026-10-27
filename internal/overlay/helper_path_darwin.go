package overlay

import "path/filepath"

func bundledHelperName(name string, backend Backend) string {
	if backend == BackendQt {
		return filepath.Join("Arcana World Elegant Overlay.app", "Contents", "MacOS", name)
	}
	return name
}
