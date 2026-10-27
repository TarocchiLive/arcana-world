//go:build !darwin

package overlay

func bundledHelperName(name string, _ Backend) string {
	return name
}
