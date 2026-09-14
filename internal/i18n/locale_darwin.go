package i18n

import (
	"context"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// AppleLanguages 是界面语言的优先列表，终端 LANG 可能只反映区域或编码设置。
func systemLanguage() string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return ""
	}
	languages := strings.FieldsFunc(string(output), func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("(),\"", r)
	})
	if len(languages) == 0 {
		return ""
	}
	return languages[0]
}
