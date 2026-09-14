//go:build !darwin

package i18n

// 其他平台沿用终端的 POSIX 语言设置。
func systemLanguage() string { return "" }
