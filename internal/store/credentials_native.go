//go:build !linux

package store

// Windows 和 macOS 自带凭据存储；访问或解锁失败不代表服务不存在。
func systemKeyringMissing() (bool, error) { return false, nil }
