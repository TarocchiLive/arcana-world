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
}
type webInteraction struct {
	account, mode, edit, prompt, confirmation string
	selection                                 *roomSelection
	qr                                        *domain.QR
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
	case "studio", "outputs", "accounts", "settings", "help":
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

func (m *Model) observeWebContext() {
	if !m.web.enabled {
		return
	}
	uid := ""
	if m.account != nil {
		uid = m.account.UID
	}
	context := webInteraction{uid, m.mode, m.editKind, m.prompt, m.confirmAction, m.selection, m.qr}
	if context != m.web.context {
		m.web.context = context
		m.web.revision++
		m.web.dialog++
		m.web.imageSource = ""
		m.web.image = ""
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
	return cmd
}
func webKey(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }
func (m *Model) webCommand(page string, c webui.Command) (tea.Cmd, error) {
	invalid := func() (tea.Cmd, error) { return nil, errors.New(i18n.T(i18n.WebActionUnavailable)) }
	if c.Action == "cancel" {
		if m.mode == "" {
			return invalid()
		}
		m.reveal = false
		if m.busy {
			if m.cancel != nil {
				m.canceled = true
				m.cancel()
			}
			m.qrGeneration++
		}
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
		switch m.mode {
		case "form":
			if c.Action == "submit" {
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
			if c.Action == "submit" || c.Action == "set:cover-path" {
				if strings.TrimSpace(c.Value) == "" {
					return invalid()
				}
				return m.prepareCover(strings.TrimSpace(c.Value)), nil
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
	if c.Action == "send" && page == "studio" && m.account != nil && m.room != nil {
		if strings.TrimSpace(c.Value) == "" {
			return invalid()
		}
		m.chatInput.SetValue(c.Value)
		return m.sendChat(), nil
	}
	return m.webAction(page, c.Action)
}
func (m *Model) webSnapshot(page string) webui.Snapshot {
	s := webui.Snapshot{Revision: m.web.revision, Language: i18n.Language(), Page: page, Busy: m.busy || m.obsBusy, Status: m.webText(m.status), Warning: m.statusWarning, OBS: m.obsState.Connected, OBSConnecting: m.obsBusy, StreamAvailable: m.stream != nil, Overlay: m.overlayEnabled, Fields: m.webFields(page), Accounts: []webui.Account{}, Help: []webui.HelpArticle{}, Messages: []webui.Message{}, Logs: []string{}}
	s.ChatDraft = m.chatInput.Value()
	if m.account != nil {
		s.Account = m.webText(m.account.Name + " · " + m.account.UID)
		s.AccountUID = m.account.UID
	}
	if m.room != nil {
		s.Room = &webui.Room{ID: m.room.ID, Title: m.webText(m.room.Title), Area: m.webText(m.room.ParentName + " / " + m.room.AreaName), Live: m.room.Live, Cover: webURL(m.room.CoverURL), Announcement: m.webText(m.room.Announcement), CoverStatus: m.webText(m.room.CoverStatus)}
	}
	if m.tts != nil {
		s.TTS = m.tts.enabled
	}
	if m.chat != nil {
		s.ChatConnected = m.chat.state.Phase == "connected" && m.account != nil && m.chat.state.AccountUID == m.account.UID
		if page == "studio" && m.room != nil && m.account != nil && m.chat.state.AccountUID == m.account.UID {
			for _, e := range m.chat.entries {
				if e.RoomID != m.room.ID || e.Deleted {
					continue
				}
				text := e.Text
				if text == "" && e.Gift != "" {
					text = fmt.Sprintf("%s × %d", e.Gift, e.Count)
				}
				if text == "" {
					text = e.Title
				}
				s.Messages = append(s.Messages, webui.Message{ID: fmt.Sprintf("%d:%d:%d", e.RoomID, e.Sequence, e.Time.UnixNano()), Time: e.Time.Format(time.RFC3339), Name: m.webText(e.User), Text: m.webText(text), Kind: m.webText(e.Kind)})
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
func (m *Model) webDialog() *webui.Interaction {
	d := &webui.Interaction{ID: strconv.FormatUint(m.web.dialog, 10), Kind: "info", Title: m.webText(m.prompt), Choices: []webui.Option{}, Target: m.webTarget(), Revision: m.web.revision}
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
		d.Text = i18n.T(i18n.WebCoverPathHint)
		if m.room != nil {
			d.Image = webURL(m.room.CoverURL)
		}
	case "cover-review":
		d.Kind = "confirm"
		d.Title = i18n.T(i18n.TUICoverConfirmTitle)
		if m.cover != nil {
			if m.web.image == "" {
				m.web.image = "data:image/png;base64," + base64.StdEncoding.EncodeToString(m.cover.PNG)
			}
			d.Image = m.web.image
			d.Text = fmt.Sprintf("%d × %d → %d × %d", m.cover.SourceWidth, m.cover.SourceHeight, coverimage.Width, coverimage.Height)
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
	return d
}
