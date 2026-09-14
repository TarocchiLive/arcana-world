package i18n

import (
	"reflect"
	"regexp"
	"testing"
)

func TestAutoLanguageSourcePrecedence(t *testing.T) {
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_MESSAGES", "zh_CN.UTF-8")
	t.Setenv("LC_ALL", "C")
	if autoEnglish("zh-Hans-CN") {
		t.Fatal("terminal locale overrode the Chinese system UI language")
	}
	if !autoEnglish("") {
		t.Fatal("LC_ALL did not take precedence when system detection was unavailable")
	}
	t.Setenv("LC_ALL", "")
	if autoEnglish("") {
		t.Fatal("LC_MESSAGES did not take precedence over LANG")
	}
	if !autoEnglish("en-US") {
		t.Fatal("terminal locale overrode the English system UI language")
	}
	t.Setenv("LC_MESSAGES", "")
	if !autoEnglish("") {
		t.Fatal("LANG was not used when higher-priority sources were unavailable")
	}
	t.Setenv("LANG", "zh@traditional")
	if autoEnglish("") {
		t.Fatal("Chinese locale with a POSIX modifier was not recognized")
	}
}

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
