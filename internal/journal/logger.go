package journal

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// minSecretLength 以下的值过于常见，替换会破坏无关字段；真实凭据均远长于此。
const minSecretLength = 8

const redacted = "[REDACTED]"

var (
	secretsMu sync.Mutex
	secrets   atomic.Pointer[[]string]
	// level 零值为 INFO：默认只记录生命周期、状态变化和失败，请求级细节需显式开启 DEBUG。
	level slog.LevelVar
	base  = slog.NewTextHandler(sink{}, &slog.HandlerOptions{Level: &level, AddSource: true, ReplaceAttr: replaceAttr})
)

// SetLevel 设置写入日志的最低级别，对已创建的 Logger 立即生效。
func SetLevel(l slog.Level) { level.Set(l) }

// Logger 返回带 component 字段的调试日志记录器，可在包初始化时创建。
// 记录写入当前打开的 Log；没有打开的日志或低于当前级别时不格式化、直接丢弃。
// 调试日志面向开发排查，消息使用固定英文短语，细节放在键值字段中。
func Logger(component string) *slog.Logger {
	return slog.New(handler{base}).With("component", component)
}

// AddSecrets 登记写入日志前需要隐藏的值，例如 Cookie、令牌、密码和推流码。
// 登记在进程内持续有效；短于 minSecretLength 的值会被忽略。
func AddSecrets(values ...string) {
	secretsMu.Lock()
	defer secretsMu.Unlock()
	var current []string
	if list := secrets.Load(); list != nil {
		current = *list
	}
	next := slices.Clone(current)
	for _, value := range values {
		if len(value) >= minSecretLength && !slices.Contains(next, value) {
			next = append(next, value)
		}
	}
	if len(next) == len(current) {
		return
	}
	// 先替换较长的值，避免较短机密是其子串时留下残片。
	slices.SortFunc(next, func(a, b string) int { return len(b) - len(a) })
	secrets.Store(&next)
}

// handler 仅在存在打开的日志时启用基础处理器。
type handler struct{ slog.Handler }

func (h handler) Enabled(ctx context.Context, level slog.Level) bool {
	return active.Load() != nil && h.Handler.Enabled(ctx, level)
}
func (h handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return handler{h.Handler.WithAttrs(attrs)}
}
func (h handler) WithGroup(name string) slog.Handler { return handler{h.Handler.WithGroup(name)} }

// sink 将 TextHandler 的单行记录转交当前日志；TextHandler 每条记录只调用一次 Write，
// 并对含换行或控制字符的值加引号转义，因此一条记录始终占一个物理行。
type sink struct{}

func (sink) Write(record []byte) (int, error) {
	if l := active.Load(); l != nil {
		l.writeRecord(record)
	}
	return len(record), nil
}

// replaceAttr 去掉处理器自带的时间字段（由 writeRecord 统一添加），并隐藏已登记的机密。
func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	list := secrets.Load()
	if list == nil {
		return a
	}
	var text string
	switch a.Value.Kind() {
	case slog.KindString:
		text = a.Value.String()
	case slog.KindAny:
		switch v := a.Value.Any().(type) {
		case error:
			text = v.Error()
		default:
			text = fmt.Sprint(v)
		}
	default:
		return a
	}
	if hidden := redact(text, *list); hidden != text {
		a.Value = slog.StringValue(hidden)
	}
	return a
}

func redact(text string, list []string) string {
	for _, secret := range list {
		if strings.Contains(text, secret) {
			text = strings.ReplaceAll(text, secret, redacted)
		}
	}
	return text
}
