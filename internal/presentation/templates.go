package presentation

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
	"gopkg.in/yaml.v3"
)

const maxTemplateFile = 256 << 10
const maxTemplateSource = 16 << 10
const maxRenderedBytes = 16 << 10

type outputTemplates struct {
	defaults map[string]*template.Template
	custom   map[string]*template.Template
	path     string
}

// TemplateSet 在加载时编译，运行错误会永久停用对应文件，直到重新加载。
// 锁同时保证多个调用者只收到一次运行错误。
type TemplateSet struct {
	mu           sync.Mutex
	english      bool
	overlay, tts outputTemplates
}

var defaultOnce [2]sync.Once
var defaultSets [2]*TemplateSet

// DefaultTemplates 返回仅包含内置值的模板集，不读写配置目录。
func DefaultTemplates(english bool) *TemplateSet {
	i := 0
	if english {
		i = 1
	}
	defaultOnce[i].Do(func() {
		s := &TemplateSet{english: english}
		s.overlay.defaults = make(map[string]*template.Template, len(specs))
		s.tts.defaults = make(map[string]*template.Template, len(specs))
		for _, spec := range specs {
			overlay, tts := spec.OverlayTemplate, spec.TTSTemplate
			if english {
				value, ok := englishTemplates[spec.ID]
				if !ok {
					panic("缺少英文默认模板：" + spec.ID)
				}
				overlay, tts = value.overlay, value.tts
			}
			s.overlay.defaults[spec.ID] = template.Must(compileTemplate(spec.ID, overlay))
			s.tts.defaults[spec.ID] = template.Must(compileTemplate(spec.ID, tts))
		}
		defaultSets[i] = s
	})
	return defaultSets[i]
}

// LoadTemplates 仅首次创建当前语言文件；任何失败均保留可用的内置模板。
func LoadTemplates(configDir string, english bool) (*TemplateSet, []error) {
	defaults := DefaultTemplates(english)
	s := &TemplateSet{english: english}
	s.overlay.defaults, s.tts.defaults = defaults.overlay.defaults, defaults.tts.defaults
	locale := localized(english, "zh-CN", "en")
	dir := filepath.Join(configDir, "templates")
	var problems []error
	for _, item := range []struct {
		name   string
		output *outputTemplates
	}{{"overlay", &s.overlay}, {"tts", &s.tts}} {
		item.output.path = filepath.Join(dir, item.name+"."+locale+".yaml")
		if err := s.load(item.output); err != nil {
			problems = append(problems, s.problem(item.output.path, err))
		}
	}
	return s, problems
}

func (s *TemplateSet) problem(path string, err error) error {
	return fmt.Errorf(localized(s.english,
		"模板文件 %s：%w；本次运行已使用内置默认模板。修正文件后重启，或删除文件后重启以重新生成。",
		"Template file %s: %w; built-in defaults are in use for this session. Fix the file and restart, or delete it and restart to regenerate it."), path, err)
}

func (s *TemplateSet) load(out *outputTemplates) error {
	// 打开命名管道会等待写入端，必须先拒绝非普通文件，避免阻塞启动。
	info, err := os.Stat(out.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && !info.Mode().IsRegular() {
		return errors.New(localized(s.english, "模板路径必须是普通文件", "template path must be a regular file"))
	}
	file, err := os.Open(out.path)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(filepath.Dir(out.path), 0700); err != nil {
			return err
		}
		file, err = os.OpenFile(out.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			_, writeErr := io.WriteString(file, s.generated(out))
			closeErr := file.Close()
			return errors.Join(writeErr, closeErr)
		}
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		file, err = os.Open(out.path)
	}
	if err != nil {
		return err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New(localized(s.english, "模板路径必须是普通文件", "template path must be a regular file"))
	}
	data, err := io.ReadAll(io.LimitReader(file, maxTemplateFile+1))
	if err != nil {
		return err
	}
	if len(data) > maxTemplateFile {
		return errors.New(localized(s.english, "文件超过 256 KiB 上限", "file exceeds the 256 KiB limit"))
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New(localized(s.english, "只允许一个 YAML 文档", "only one YAML document is allowed"))
	}
	if len(document.Content) == 0 {
		return nil
	}
	root := document.Content[0]
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" && root.Value == "" {
		return nil
	}
	if root.Kind != yaml.MappingNode || root.Tag != "!!map" || root.Anchor != "" {
		return fmt.Errorf(localized(s.english, "第 %d 行：需要事件与字符串的映射", "line %d: expected an event-to-string mapping"), root.Line)
	}
	custom := make(map[string]*template.Template, len(root.Content)/2)
	seen := make(map[string]bool, len(root.Content)/2)
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		id := key.Value
		fail := func(reason string) error {
			return fmt.Errorf(localized(s.english, "事件 %q，第 %d 行：%s", "event %q, line %d: %s"), id, value.Line, reason)
		}
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Anchor != "" || !knownEvent(id) {
			return fail(localized(s.english, "未知或无效的事件标识", "unknown or invalid event ID"))
		}
		if seen[id] {
			return fail(localized(s.english, "重复的事件标识", "duplicate event ID"))
		}
		seen[id] = true
		if value.Kind == yaml.ScalarNode && value.Tag == "!!null" && value.Value == "" && value.Anchor == "" {
			continue
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" || value.Anchor != "" {
			return fail(localized(s.english, "模板必须是字符串（不支持锚点或别名）", "template must be a string (anchors and aliases are not supported)"))
		}
		if strings.TrimSpace(value.Value) == "" {
			continue
		}
		t, err := compileTemplate(id, value.Value)
		if err != nil {
			return fail(localized(s.english, "无效或不安全的 Go 模板：", "invalid or unsafe Go template: ") + err.Error())
		}
		if err := validateExecution(t, id, s.english); err != nil {
			return fail(localized(s.english, "模板试运行失败：", "template validation failed: ") + err.Error())
		}
		custom[id] = t
	}
	out.custom = custom
	return nil
}

func (s *TemplateSet) generated(out *outputTemplates) string {
	var b strings.Builder
	b.WriteString(localized(s.english,
		"# 浮层或 TTS 事件模板\n# 直接修改模板内容，保存后重启生效；未配置或留空的事件使用内置默认。\n# 配置有误时会提示并使用内置默认；删除本文件后重启可重新生成。\n# Go 模板文档：https://pkg.go.dev/text/template\n# 支持 if、with、比较、布尔运算及 len；不支持 range、define、template、变量或任意方法。\n# 时间示例：{{.Time.Format \"15:04:05\"}}；with 内可用 $.User 访问根字段。\n# .User：用户名；.Text：正文；.Gift：礼物名；.Count：数量。\n# .Amount：金额（SC 为元，礼物为原始金/银瓜子）；.Coins：带单位的礼物金额。\n# .Guard：大航海等级；.GuardDuration：大航海时长；.Silent：禁言规则。\n",
		"# Overlay or TTS event templates\n# Edit template values, then restart; missing or empty events use built-in defaults.\n# Invalid files trigger a warning and default output; delete this file and restart to regenerate it.\n# Go template documentation: https://pkg.go.dev/text/template\n# Supports if, with, comparisons, boolean operations and len; no range, define, template, variables or arbitrary methods.\n# Time example: {{.Time.Format \"15:04:05\"}}; use $.User inside with for root fields.\n# .User: username; .Text: message; .Gift: gift name; .Count: quantity.\n# .Amount: amount (yuan for SC, original gold/silver seeds for gifts); .Coins: gift amount with unit.\n# .Guard: membership tier; .GuardDuration: membership duration; .Silent: chat restriction.\n"))
	b.WriteString(localized(s.english, "# 通用字段：", "# Common fields: ") + publicFields + "\n")
	for _, spec := range specs {
		b.WriteString("\n# " + i18n.ForLanguage(localized(s.english, "zh-CN", "en"), spec.Label))
		b.WriteString("\n# " + localized(s.english, "常用字段：", "Typical fields: ") + eventFieldHint(spec.ID))
		if methods := detailFields[spec.ID]; len(methods) != 0 {
			for _, field := range methods {
				b.WriteString("; ." + field.method + " " + strconv.Quote(field.name))
				if field.method == "F" {
					b.WriteString(" " + strconv.Quote(localized(s.english, "默认文字", "fallback")))
				}
			}
		}
		b.WriteByte('\n')
		b.WriteString(spec.ID + ": '" + strings.ReplaceAll(out.defaults[spec.ID].Tree.Root.String(), "'", "''") + "'\n")
	}
	return b.String()
}

// RenderOverlay 与 RenderTTS 在执行失败后丢弃部分输出，并回退整个对应文件。
func (s *TemplateSet) RenderOverlay(e danmaku.Event) (string, error) {
	if s == nil {
		s = DefaultTemplates(i18n.Language() == "en")
	}
	return s.render(e, &s.overlay, 2048)
}

func (s *TemplateSet) RenderTTS(e danmaku.Event) (string, error) {
	if s == nil {
		s = DefaultTemplates(i18n.Language() == "en")
	}
	return s.render(e, &s.tts, 500)
}

func (s *TemplateSet) render(e danmaku.Event, out *outputTemplates, limit int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := ID(e)
	base := out.defaults[id]
	if base == nil {
		return "", nil
	}
	selected := out.custom[id]
	if selected == nil {
		selected = base
	}
	data := eventData{Event: &e, english: s.english}
	var b boundedOutput
	err := selected.Execute(&b, data)
	if err == nil {
		return clean(b.String(), limit), nil
	}
	if selected == base {
		return "", s.problem(out.path, err)
	}
	out.custom = nil
	b.Reset()
	fallbackErr := base.Execute(&b, data)
	if fallbackErr != nil {
		return "", s.problem(out.path, errors.Join(err, fallbackErr))
	}
	return clean(b.String(), limit), s.problem(out.path, err)
}

type boundedOutput struct{ strings.Builder }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > maxRenderedBytes-b.Len() {
		return 0, errors.New("rendered output exceeds 16 KiB")
	}
	return b.Builder.Write(p)
}
