package tui

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"arcana-world/internal/coverimage"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	"arcana-world/internal/termimage"
	"arcana-world/internal/webui"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/skip2/go-qrcode"
)

// 浏览器请求与业务结果共用事件循环；HTTP 协程不读取模型。
type webState struct {
	enabled     bool
	revision    uint64
	dialog      uint64
	context     webInteraction
	imageSource string
	image       string
	imageAvatar *termimage.Thumbnail
	// 最近一次输出页请求时间；多个浏览器同时打开时，只要有一个停留在输出页就保留示例。
	outputsSeen  time.Time
	previewCheck bool
}

// 所有浏览器停止轮询输出页超过租期后，示例预览自动结束。
const webPreviewLease = 3 * time.Second

type webPreviewCheck struct{}
type webInteraction struct {
	account, mode, edit, prompt, confirmation string
	selection                                 *roomSelection
	qr                                        *domain.QR
	chatContext                               webChatInteraction
}
type webRequest struct {
	ctx     context.Context
	page    string
	command *webui.Command
	reply   chan webReply
}
type webReply struct {
	snapshot webui.Snapshot
	err      error
}

func (m *Model) EnableWeb() { m.web.enabled = true; m.web.revision = 1 }

func (m *Model) WebRequest(ctx context.Context, send func(tea.Msg), page string, command *webui.Command) (webui.Snapshot, error) {
	request := webRequest{ctx: ctx, page: page, reply: make(chan webReply, 1)}
	if command != nil {
		copied := *command
		request.command = &copied
	}
	select {
	case <-ctx.Done():
		return webui.Snapshot{}, ctx.Err()
	default:
	}
	send(request)
	select {
	case result := <-request.reply:
		return result.snapshot, result.err
	case <-ctx.Done():
		return webui.Snapshot{}, ctx.Err()
	case <-m.ctx.Done():
		return webui.Snapshot{}, m.ctx.Err()
	}
}

func webPage(id string) (string, bool) {
	switch id {
	case "studio", "chat", "outputs", "accounts", "settings", "help":
		return id, true
	}
	return "", false
}
func (m *Model) webText(s string) string {
	s = m.safe(ansi.Strip(s))
	if proxy, err := url.Parse(m.config.Proxy); err == nil && proxy.User != nil {
		s = strings.ReplaceAll(s, proxy.User.String(), i18n.T(i18n.TUISecurityHidden))
	}
	return s
}
func webURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return ""
	}
	return u.String()
}

func webCoverURL(s string) string {
	// 封面 CDN 支持 HTTPS；统一旧式地址以满足页面图片安全策略。
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "//") {
		s = "https:" + s
	} else if strings.HasPrefix(s, "http://") {
		s = "https://" + strings.TrimPrefix(s, "http://")
	}
	return webURL(s)
}

func (m *Model) observeWebContext() {
	if !m.web.enabled {
		return
	}
	uid := ""
	if m.account != nil {
		uid = m.account.UID
	}
	context := webInteraction{uid, m.mode, m.editKind, m.prompt, m.confirmAction, m.selection, m.qr, m.webChatContext()}
	if context != m.web.context {
		previous := m.web.context
		m.web.context = context
		m.web.revision++
		previous.chatContext = previous.chatContext.dialogIdentity()
		context.chatContext = context.chatContext.dialogIdentity()
		if context != previous {
			m.web.dialog++
		}
		m.web.imageSource = ""
		m.web.image = ""
		m.web.imageAvatar = nil
	}
}
func (m *Model) handleWebRequest(r webRequest) tea.Cmd {
	if err := r.ctx.Err(); err != nil {
		r.reply <- webReply{err: err}
		return nil
	}
	page, ok := webPage(r.page)
	if !ok {
		r.reply <- webReply{err: errors.New(i18n.T(i18n.WebRequestInvalid))}
		return nil
	}
	if page == "outputs" {
		m.web.outputsSeen = time.Now()
	}
	m.expireWebPreview()
	m.observeWebContext()
	var cmd tea.Cmd
	var err error
	if r.command != nil {
		c := r.command
		if c.Revision != m.web.revision {
			err = errors.New(i18n.T(i18n.WebInteractionChanged))
		} else if m.busy && c.Action != "cancel" {
			err = errors.New(i18n.T(i18n.WebOperationBusy))
		} else {
			cmd, err = m.webCommand(page, *c)
			if err == nil {
				m.web.revision++
			}
		}
		m.observeWebContext()
		m.syncTTS()
		m.publishOverlay()
	}
	r.reply <- webReply{snapshot: m.webSnapshot(page), err: err}
	return tea.Batch(cmd, m.scheduleWebPreviewCheck())
}

// 浏览器离开输出页、隐藏或关闭后不再轮询，租期到达时与终端离开浮层页一样恢复实际内容。
func (m *Model) expireWebPreview() {
	if m.overlay != nil && m.overlay.preview && time.Since(m.web.outputsSeen) > webPreviewLease {
		m.overlay.preview = false
		m.publishOverlay()
	}
}
func (m *Model) scheduleWebPreviewCheck() tea.Cmd {
	if m.web.previewCheck || m.overlay == nil || !m.overlay.preview {
		return nil
	}
	m.web.previewCheck = true
	return tea.Tick(webPreviewLease, func(time.Time) tea.Msg { return webPreviewCheck{} })
}
func (m *Model) handleWebPreviewCheck() tea.Cmd {
	m.web.previewCheck = false
	m.expireWebPreview()
	return m.scheduleWebPreviewCheck()
}
func webKey(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }
func (m *Model) webCommand(page string, c webui.Command) (tea.Cmd, error) {
	invalid := func() (tea.Cmd, error) { return nil, errors.New(i18n.T(i18n.WebActionUnavailable)) }
	if c.Action == "cancel" {
		if m.mode == "" {
			return invalid()
		}
		// 尚未提交的确认只退回上一层；执行中的取消仍清理整个操作。
		if !m.busy {
			if m.mode == "confirm" && m.webChatValid() &&
				(m.confirmAction == "speaker-apply" || m.confirmAction == "moderation-apply") {
				return m.modalKey(webKey(tea.KeyEsc)), nil
			}
			if m.mode == "cover-review" {
				return m.coverKey(webKey(tea.KeyEsc)), nil
			}
			if m.mode == "confirm" && m.confirmAction == "login-switch" && m.pendingAccount != nil {
				m.mode, m.confirmAction = "login-save", ""
				return nil, nil
			}
		}
		m.reveal = false
		if m.busy {
			if m.cancel != nil {
				logger.Info("operation cancel requested", "id", m.operation)
				m.canceled = true
				m.cancel()
			}
			m.qrGeneration++
		}
		m.clearWebChat()
		m.mode = ""
		m.selection = nil
		m.cover = nil
		m.qr = nil
		m.qrText = ""
		m.faceURL = ""
		m.pendingAccount = nil
		m.overlaySettings = nil
		m.clearOverlayAppearancePreview()
		m.input.Reset()
		m.input.Blur()
		m.setStatus(i18n.T(i18n.TUIStatusOperationCanceled))
		return nil, nil
	}
	if m.mode != "" {
		if strings.HasPrefix(m.webTarget(), "chat-") {
			if page != "chat" || !m.webChatValid() {
				return invalid()
			}
			if c.Action == "back" && m.webChatBackAvailable() {
				return m.modalKey(webKey(tea.KeyEsc)), nil
			}
			if m.mode == "members" && c.Action == "members-refresh" {
				return m.refreshRoomMembers(), nil
			}
			if m.mode == "speaker" {
				return m.webSpeakerAction(c.Action)
			}
		}
		switch m.mode {
		case "form":
			if c.Action == "submit" && m.editKind != "cover-path" {
				return m.webSubmit(c.Value)
			}
		case "pick":
			if c.Action == "choose" {
				for i, ch := range m.choices {
					if ch.value == c.Value {
						m.selected = i
						return m.choose(), nil
					}
				}
			}
		case "confirm":
			if c.Action == "confirm" {
				m.selected = 1
				// 保留管理弹层直到结果到达，允许取消并清除异步回调的目标。
				if m.confirmAction == "moderation-apply" {
					if !m.moderationValid(m.moderation) || !m.moderation.pending {
						return invalid()
					}
					return m.applyModeration(), nil
				}
				if m.confirmAction == "speaker-apply" {
					if !m.speakerValid(m.speaker) || !m.speaker.pending {
						return invalid()
					}
					return m.applyChatSpeaker(), nil
				}
				return m.modalKey(webKey(tea.KeyEnter)), nil
			}
		case "selection":
			s := m.selection
			if s == nil {
				return invalid()
			}
			if c.Action == "search" {
				s.query = []rune(selectionText(c.Value))
				s.parent = ""
				s.rebuild()
				return nil, nil
			}
			if c.Action == "choose" {
				i, e := strconv.Atoi(c.Value)
				if e == nil && i >= 0 && i < len(s.items) {
					s.selected = i
					return m.updateSelection(webKey(tea.KeyEnter)), nil
				}
			}
		case "cover":
			if page == "studio" && c.Action == "cover-upload" && m.account != nil && m.room != nil {
				return work(m, coverPrepareOperation(), func(ctx context.Context) (*coverimage.Prepared, error) {
					return coverimage.PrepareUpload(ctx, c.Value)
				}), nil
			}
		case "cover-review":
			if c.Action == "confirm" {
				return m.coverKey(webKey(tea.KeyEnter)), nil
			}
		case "login-save":
			if c.Action == "confirm" && m.pendingAccount != nil {
				return m.saveLogin(*m.pendingAccount), nil
			}
		}
		return invalid()
	}
	if strings.HasPrefix(c.Action, "set:") {
		return m.webSetField(page, strings.TrimPrefix(c.Action, "set:"), c.Value)
	}
	if c.Action == "reveal" && page == "studio" && m.stream != nil {
		m.mode = "web-stream"
		return nil, nil
	}
	if c.Action == "send" && page == "chat" && m.account != nil && m.room != nil {
		if strings.TrimSpace(c.Value) == "" {
			return invalid()
		}
		m.chatInput.SetValue(c.Value)
		return m.sendChat(), nil
	}
	if page == "chat" && !slices.Contains(webSessionActions, c.Action) {
		return m.webChatAction(c)
	}
	return m.webAction(page, c.Action)
}
func (m *Model) webSnapshot(page string) webui.Snapshot {
	s := webui.Snapshot{Revision: m.web.revision, Language: i18n.Language(), Page: page, Busy: m.busy || m.obsBusy, Status: m.webText(m.status), Warning: m.statusWarning, OBS: m.obsState.Connected, OBSConnecting: m.obsBusy, StreamAvailable: m.stream != nil, Overlay: m.overlayEnabled, Fields: m.webFields(page), Accounts: []webui.Account{}, Help: []webui.HelpArticle{}, Messages: []webui.Message{}, Logs: []string{}}
	if m.notificationVisible() {
		s.Notification = &webui.Notification{
			Revision: m.notification.generation,
			Text:     m.webText(m.notification.text),
			Warning:  m.notification.warning,
		}
	}
	if page == "outputs" {
		provider := m.ttsProvider()
		s.TTSSettingsTitle = i18n.T(provider.SettingsTitle)
		if provider.Model {
			s.TTSModelText = m.webText(m.ttsModelText())
			for _, action := range m.ttsModelActions() {
				s.TTSModelActions = append(s.TTSModelActions, webui.Option{ID: action.value, Label: action.label})
			}
		}
	}
	if page == "chat" {
		s.Audience = m.webText(m.audienceText())
	}
	if m.account != nil {
		s.Account = m.webText(m.account.Name + " · " + m.account.UID)
		s.AccountUID = m.account.UID
	}
	if m.room != nil {
		s.Room = &webui.Room{ID: m.room.ID, Title: m.webText(m.room.Title), Area: m.webText(m.room.ParentName + " / " + m.room.AreaName), Live: m.room.Live, Cover: webCoverURL(m.room.CoverURL), Announcement: m.webText(m.room.Announcement), CoverStatus: m.webText(m.room.CoverStatus)}
	}
	if m.tts != nil {
		s.TTS = m.tts.enabled
	}
	if m.chat != nil {
		s.ChatConnected = m.chat.state.Phase == "connected" && m.account != nil && m.chat.state.AccountUID == m.account.UID
		if page == "chat" && m.room != nil && m.account != nil && m.chat.state.AccountUID == m.account.UID {
			// 与终端共用事件文案，并按时间从旧到新显示。
			for index := len(m.chat.entries) - 1; index >= 0; index-- {
				e := m.chat.entries[index]
				if e.RoomID != m.room.ID {
					continue
				}
				text := chatEventText(e)
				if e.Kind == "chat" {
					text = strings.TrimPrefix(text, chatText(e.User, 128)+"：")
				}
				uid := e.UID
				if e.Mystery || uid == "0" {
					uid = ""
				}
				s.Messages = append(s.Messages, webui.Message{
					ID:   fmt.Sprintf("%d:%d:%d", e.RoomID, e.Sequence, e.Time.UnixNano()),
					Time: e.Time.Format(time.RFC3339), Name: m.webText(chatText(e.User, 128)),
					Text: m.webText(text), Kind: m.webText(e.Kind), UID: m.webText(uid),
					MedalName: m.webText(chatText(e.MedalName, 128)), MedalLevel: e.MedalLevel,
					Mystery: e.Mystery, Deleted: e.Deleted,
				})
			}
		}
	}
	switch page {
	case "accounts":
		for _, a := range m.store.Accounts() {
			s.Accounts = append(s.Accounts, webui.Account{UID: a.UID, Name: m.webText(a.Name), Active: a.UID == s.AccountUID})
		}
	case "help":
		s.Help = webHelpArticles()
	case "settings":
		for _, line := range m.logs {
			s.Logs = append(s.Logs, m.webText(line))
		}
	}
	if m.mode != "" {
		s.Interaction = m.webDialog()
	}
	return s
}
func (m *Model) webConfirmationLabel() string {
	switch m.mode {
	case "cover-review":
		return i18n.T(i18n.TUIOperationUploadCover)
	case "login-save":
		return i18n.T(i18n.WebSaveLogin)
	}
	switch m.confirmAction {
	case "speaker-apply":
		if m.speakerValid(m.speaker) && m.speaker.action >= 0 && m.speaker.action < 3 {
			return i18n.T(speakerActionLabel(m.speaker.action))
		}
	case "moderation-apply":
		if m.moderationValid(m.moderation) {
			return i18n.T(moderationActionLabel(m.moderation.action))
		}
	case "start":
		return i18n.T(i18n.TUIMenuStartLive)
	case "stop":
		return i18n.T(i18n.TUIMenuStopLive)
	case "obs-disconnect":
		return i18n.T(i18n.TUIOBSDisconnect)
	case "settings-reset":
		return i18n.T(i18n.TUISettingsReset)
	case "overlay-restore":
		return i18n.T(i18n.TUIOverlayRestore)
	case "chat-disable":
		return i18n.T(i18n.WebStopListening)
	case "login-switch":
		return i18n.T(i18n.WebSwitchAccount)
	}
	if strings.HasPrefix(m.confirmAction, "switch:") {
		return i18n.T(i18n.WebSwitchAccount)
	}
	if strings.HasPrefix(m.confirmAction, "delete:") {
		return i18n.T(i18n.TUIOperationRemoveAccount)
	}
	return i18n.T(i18n.TUIConfirmExecuteLabel)
}

func (m *Model) webDialog() *webui.Interaction {
	d := &webui.Interaction{ID: strconv.FormatUint(m.web.dialog, 10), Kind: "info", Title: m.webText(m.prompt), Choices: []webui.Option{}, Target: m.webTarget(), Revision: m.web.revision}
	d.Back = m.webChatBackAvailable()
	if m.webChatDialog(d) {
		return d
	}
	switch m.mode {
	case "web-stream":
		d.Title = i18n.T(i18n.WebStreamCredentials)
		if m.stream != nil {
			d.Text = clean(ansi.Strip(m.stream.Address + "\n" + m.stream.Key))
		}
	case "form":
		d.Kind = "form"
		d.Value = m.webText(m.input.Value())
		d.Secret = m.input.EchoMode == textinput.EchoPassword
		if d.Secret || m.editKind == "proxy" {
			d.Value = ""
		}
	case "pick":
		d.Kind = "pick"
		for _, ch := range m.choices {
			selected := false
			if m.editKind == "overlay-displays" {
				selected = slices.Contains(m.config.Overlay.Displays, ch.value)
				if m.config.Overlay.Displays == nil {
					selected = slices.ContainsFunc(m.overlayDisplays, func(display overlay.Display) bool { return display.ID == ch.value && display.Selected })
				}
			}
			d.Choices = append(d.Choices, webui.Option{ID: ch.value, Label: m.webText(ch.label), Selected: selected})
		}
	case "confirm":
		d.Kind = "confirm"
		d.Text = m.webText(m.prompt)
	case "selection":
		if s := m.selection; s != nil {
			d.Kind = "selection"
			d.Title = i18n.T(i18n.TUIMenuSelectCategory)
			d.Value = string(s.query)
			for i, item := range s.items {
				label := item.parent
				if item.area != nil {
					label = item.area.Parent + " / " + item.area.Name
				}
				d.Choices = append(d.Choices, webui.Option{ID: strconv.Itoa(i), Label: m.webText(label)})
			}
		}
	case "cover":
		d.Kind = "cover"
		d.Title = i18n.T(i18n.TUICoverTitle)
		d.Text = i18n.T(i18n.WebCoverUploadHint)
		if m.room != nil {
			d.Image = webCoverURL(m.room.CoverURL)
		}
	case "cover-review":
		d.Kind = "confirm"
		d.Title = i18n.T(i18n.TUICoverConfirmTitle)
		if m.cover != nil {
			if m.web.image == "" {
				m.web.image = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(m.cover.JPEG)
			}
			d.Image = m.web.image
			d.Text = fmt.Sprintf("%d × %d · JPEG · %.0f KiB", coverimage.Width, coverimage.Height, float64(len(m.cover.JPEG))/1024)
		}
	case "qr", "face":
		d.Kind = "qr"
		link := m.faceURL
		if m.mode == "qr" && m.qr != nil {
			link = m.qr.URL
		}
		d.URL = webURL(link)
		d.Title = i18n.T(i18n.WebScanVerify)
		if link != m.web.imageSource {
			m.web.imageSource = link
			m.web.image = ""
			if image, err := qrcode.Encode(link, qrcode.Medium, 256); err == nil {
				m.web.image = "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
			}
		}
		d.Image = m.web.image
	case "login-save":
		d.Kind = "confirm"
		d.Title = i18n.T(i18n.WebSaveLogin)
		if m.pendingAccount != nil {
			d.Text = m.webText(m.pendingAccount.Name + " · " + m.pendingAccount.UID)
		}
	}
	if d.Kind == "confirm" {
		d.Title = m.webConfirmationLabel()
		d.Choices = []webui.Option{{ID: "confirm", Label: d.Title}}
	}
	return d
}
