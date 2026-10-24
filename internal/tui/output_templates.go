package tui

import (
	"errors"

	"arcana-world/internal/i18n"
	"arcana-world/internal/presentation"
)

// 模板仅在启动和语言变化时读取，不监听用户文件，也不重置语音消费进度。
func (m *Model) syncOutputTemplates() {
	language := i18n.Language()
	if m.store == nil || m.templateLanguage == language {
		return
	}
	m.templateLanguage = language
	templates, problems := presentation.LoadTemplates(m.store.Dir(), language == "en")
	m.outputTemplates = templates
	m.overlayChat = overlayChatState{}
	m.frame = renderFrame{}
	if err := errors.Join(problems...); err != nil {
		logger.Warn("output templates load failed", "language", language, "err", err)
		m.warnOutputTemplates(err)
	}
}

func (m *Model) warnOutputTemplates(err error) {
	prior := ""
	if m.notification.persistent && !m.notification.dismissed {
		prior = m.notification.text + "\n\n"
	}
	m.warn(err.Error())
	m.notification.text = prior + m.safe(err.Error())
	m.notification.persistent = true
	m.noticeLayer = floatingLayer{}
}
