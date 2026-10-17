package tui

import (
	"arcana-world/internal/i18n"
	"arcana-world/internal/webui"
)

// 帮助是独立页面的数据，不创建交互模式，也不阻塞直播操作。
func webHelpArticles() []webui.HelpArticle {
	topics := [...]struct {
		id                   string
		title, summary, body i18n.Key
	}{
		{"start", i18n.WebHelpStartTitle, i18n.WebHelpStartSummary, i18n.WebHelpStartBody},
		{"obs", i18n.WebHelpOBSTitle, i18n.WebHelpOBSSummary, i18n.WebHelpOBSBody},
		{"room", i18n.WebHelpRoomTitle, i18n.WebHelpRoomSummary, i18n.WebHelpRoomBody},
		{"chat", i18n.WebHelpChatTitle, i18n.WebHelpChatSummary, i18n.WebHelpChatBody},
		{"outputs", i18n.WebHelpOutputsTitle, i18n.WebHelpOutputsSummary, i18n.WebHelpOutputsBody},
		{"privacy", i18n.WebHelpPrivacyTitle, i18n.WebHelpPrivacySummary, i18n.WebHelpPrivacyBody},
		{"troubleshooting", i18n.WebHelpTroubleshootingTitle, i18n.WebHelpTroubleshootingSummary, i18n.WebHelpTroubleshootingBody},
	}
	articles := make([]webui.HelpArticle, 0, len(topics))
	for _, topic := range topics {
		articles = append(articles, webui.HelpArticle{ID: topic.id, Title: i18n.T(topic.title), Summary: i18n.T(topic.summary), Body: i18n.T(topic.body)})
	}
	return articles
}
