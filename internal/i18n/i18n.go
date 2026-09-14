// Package i18n 提供进程级中英文文案选择；语言应在启动界面前设置。
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
)

var english atomic.Bool

// Key 是与具体文案解耦的消息标识；业务代码使用 messages_*.go 中的常量。
// 新增消息时同步添加两种语言的模块 JSON 条目；修改措辞只改 JSON，不改键。
// 格式串保留参数类型、顺序和 %w 错误包装，由调用方负责格式化。
type Key string

// 词表随二进制分发，不依赖当前工作目录或运行时文件。
//
//go:embed locales/*/*.json
var localeFiles embed.FS

var (
	chineseCatalog = loadCatalog("zh-CN")
	englishCatalog = loadCatalog("en")
)

// T 返回当前语言的文案；缺译时回退中文，未知消息键原样返回以便定位问题。
// 只查找启动时加载的只读词表，不解析模板，也不处理用户内容。
func T(key Key) string {
	if english.Load() {
		if text, ok := englishCatalog[key]; ok {
			return text
		}
	}
	if text, ok := chineseCatalog[key]; ok {
		return text
	}
	return string(key)
}

func loadCatalog(language string) map[Key]string {
	catalog := make(map[Key]string)
	files, err := localeFiles.ReadDir("locales/" + language)
	if err != nil {
		panic(fmt.Errorf("i18n: %s: %w", language, err))
	}
	for _, file := range files {
		name := "locales/" + language + "/" + file.Name()
		raw, err := localeFiles.Open(name)
		if err != nil {
			panic(fmt.Errorf("i18n: %s: %w", name, err))
		}
		err = decodeCatalog(raw, catalog)
		raw.Close()
		if err != nil {
			panic(fmt.Errorf("i18n: %s: %w", name, err))
		}
	}
	return catalog
}

// 逐个读取成员，避免 JSON 重复键被静默覆盖；跨模块重复键同样拒绝。
func decodeCatalog(r io.Reader, catalog map[Key]string) error {
	decoder := json.NewDecoder(r)
	first, err := decoder.Token()
	if err != nil {
		return err
	}
	if first != json.Delim('{') {
		return fmt.Errorf("词表必须是 JSON 对象")
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key := Key(token.(string))
		if _, exists := catalog[key]; exists {
			return fmt.Errorf("重复消息键 %q", key)
		}
		var text string
		if err := decoder.Decode(&text); err != nil {
			return err
		}
		if key == "" || text == "" {
			return fmt.Errorf("消息键与译文不能为空：%q", key)
		}
		catalog[key] = text
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("词表对象之后存在额外内容")
	}
	return nil
}

// SetLanguage 设置显示语言；auto 优先读取系统界面语言，再回退到终端环境变量。
// 不支持的显式语言返回错误，且不改变当前设置。
func SetLanguage(value string) error {
	language := strings.ToLower(strings.TrimSpace(value))
	if language == "auto" || language == "" {
		english.Store(autoEnglish(systemLanguage()))
		return nil
	}
	switch language {
	case "zh", "zh-cn", "zh_cn":
		english.Store(false)
	case "en", "en-us", "en_us", "en-gb", "en_gb":
		english.Store(true)
	default:
		return fmt.Errorf(T(LanguageUnsupported), value)
	}
	return nil
}

func autoEnglish(preferred string) bool {
	language := strings.TrimSpace(preferred)
	if language == "" {
		language = "zh-CN"
		for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
			if value := strings.TrimSpace(os.Getenv(name)); value != "" {
				language = value
				break
			}
		}
	}
	base := strings.ToLower(language)
	if index := strings.IndexAny(base, "_-.@"); index >= 0 {
		base = base[:index]
	}
	return base != "zh"
}
