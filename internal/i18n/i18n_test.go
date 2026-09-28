package i18n

import (
	"reflect"
	"regexp"
	"testing"
)

func TestCatalogTranslationsAndFormats(t *testing.T) {
	// 同一个格式串的参数类型、顺序和精度必须兼容；%% 不消耗参数。
	directive := regexp.MustCompile(`%(\[[1-9][0-9]*\])?[-+# 0]*([0-9]+|\*)?(\.([0-9]+|\*))?([a-zA-Z%])`)
	for key, zh := range chineseCatalog {
		en, ok := englishCatalog[key]
		if !ok {
			t.Errorf("English translation missing: %s", key)
			continue
		}
		if !reflect.DeepEqual(directive.FindAllString(zh, -1), directive.FindAllString(en, -1)) {
			t.Errorf("incompatible format arguments: %s", key)
		}
	}
	for key := range englishCatalog {
		if _, ok := chineseCatalog[key]; !ok {
			t.Errorf("Chinese translation missing: %s", key)
		}
	}
}
