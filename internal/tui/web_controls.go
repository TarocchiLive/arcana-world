package tui

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	"arcana-world/internal/presentation"
	"arcana-world/internal/tts"
	"arcana-world/internal/webui"
	tea "charm.land/bubbletea/v2"
)

// 浏览器字段和动作独立于终端菜单；保存仍复用原有业务操作。
func (m *Model) webFields(page string) []webui.Field {
	fields := []webui.Field{}
	add := func(id, group string, label i18n.Key, kind, value string, disabled bool) *webui.Field {
		fields = append(fields, webui.Field{ID: id, Group: group, Label: i18n.T(label), Kind: kind, Value: value, Disabled: disabled})
		return &fields[len(fields)-1]
	}
	boolean := strconv.FormatBool
	o := m.store.Overrides()
	if page == "studio" {
		title, announcement := "", ""
		if m.room != nil {
			title, announcement = m.room.Title, m.room.Announcement
		}
		field := add("title", "room", i18n.WebFieldTitle, "text", title, m.room == nil || m.account == nil)
		for _, recent := range m.config.RecentTitles {
			field.Options = append(field.Options, webui.Option{ID: recent, Label: recent})
		}
		add("announcement", "room", i18n.WebFieldAnnouncement, "text", announcement, m.room == nil || m.account == nil)
	}
	if page == "studio" || page == "settings" {
		add("obs-url", "connection", i18n.WebFieldOBSURL, "text", m.config.OBSURL, m.obsBusy)
		add("obs-password", "connection", i18n.WebFieldOBSPassword, "password", "", m.obsBusy)
		add("obs-auto-connect", "connection", i18n.WebFieldOBSAutoConnect, "switch", boolean(m.config.OBSAutoConnect), o.OBSAutoConnect != nil || m.obsBusy)
		add("obs-auto-stream", "connection", i18n.WebFieldOBSAutoStream, "switch", boolean(m.config.OBSAutoStream), o.OBSAutoStream != nil || m.obsBusy)
		f := add("protocol", "connection", i18n.WebStreamProtocol, "select", m.config.Protocol, false)
		f.Options = []webui.Option{{ID: "rtmp", Label: "RTMP"}, {ID: "srt-fallback", Label: i18n.T(i18n.TUIProtocolSRTFallback)}, {ID: "srt", Label: i18n.T(i18n.TUIProtocolSRTOnly)}}
	}
	if page == "settings" {
		proxy := m.config.Proxy
		hidden := false
		if u, err := url.Parse(proxy); err == nil && u.User != nil {
			proxy = ""
			hidden = true
		}
		f := add("proxy", "connection", i18n.WebFieldProxy, "text", proxy, o.Proxy != nil)
		if hidden {
			f.Label += " (" + i18n.T(i18n.TUISecurityHidden) + ")"
		}
		add("chat-enabled", "behavior", i18n.WebFieldChatEnabled, "switch", boolean(!m.config.DanmakuDisabled), false)
		add("chat-other", "behavior", i18n.WebFieldChatOther, "switch", boolean(m.config.DanmakuShowOther), false)
		f = add("chat-limit", "behavior", i18n.WebFieldChatLimit, "number", strconv.Itoa(m.config.DanmakuLimit), false)
		f.Min, f.Max, f.Step = 1, 1000, 1
		add("exit-obs-stop", "behavior", i18n.WebFieldExitOBSStop, "switch", boolean(!m.config.ExitOBSStopDisabled), false)
		add("exit-live-stop", "behavior", i18n.WebFieldExitLiveStop, "switch", boolean(!m.config.ExitLiveStopDisabled), false)
	}
	if page == "outputs" {
		add("overlay-enabled", "overlay", i18n.WebFieldOverlayEnabled, "switch", boolean(m.overlayEnabled), false)
		add("overlay-preview", "overlay", i18n.TUIOverlayPreview, "switch",
			boolean(m.overlay != nil && m.overlay.preview),
			!m.overlayEnabled || m.overlay == nil || m.overlay.state != "running")
		enabled := false
		if m.tts != nil {
			enabled = m.tts.enabled
		}
		add("tts-enabled", "tts", i18n.WebFieldTTSEnabled, "switch", boolean(enabled), m.ttsOverride != nil)
		s := m.config.Overlay
		backend := add("overlay-backend", "overlay", i18n.TUIOverlayBackend, "select", string(s.Backend), false)
		for _, value := range []overlay.Backend{overlay.BackendNative, overlay.BackendQt} {
			backend.Options = append(backend.Options, webui.Option{ID: string(value), Label: overlayBackendLabel(value)})
		}
		for _, field := range overlayFields {
			if field.group >= overlaySettingsQtText && s.Backend != overlay.BackendQt {
				continue
			}
			kind, group, value := "number", "overlay-layout", field.read(s)
			if field.group == overlaySettingsText || field.group == overlaySettingsQtText {
				group = "overlay-type"
			}
			if field.color != nil || field.qtColor != nil || strings.HasSuffix(field.key, "-alpha") {
				group = "overlay-color"
			}
			if field.color != nil || field.qtColor != nil {
				kind = "color"
			}
			if field.key == "font-family" {
				kind = "text"
			}
			switch field.key {
			case "content":
				kind, value = "select", s.Content
			case "anchor":
				kind, value = "select", string(s.Position.Anchor)
			case "italic":
				kind, value = "switch", boolean(s.Font.Italic)
			case "outline":
				kind, value = "switch", boolean(s.Outline)
			case "qt-align":
				kind, value = "select", s.Qt.Alignment
			case "qt-gradient":
				kind, value = "switch", boolean(s.Qt.Gradient)
			case "qt-animation":
				kind, value = "switch", boolean(s.Qt.AnimationEnabled)
			}
			f := add("overlay-"+field.key, group, field.label, kind, value, false)
			f.Step = field.step
			if field.key == "qt-align" {
				for _, alignment := range []string{"left", "center", "right"} {
					f.Options = append(f.Options, webui.Option{ID: alignment, Label: overlayQtAlignmentLabel(alignment)})
				}
			}
			if field.key == "font-weight" {
				f.Step = 1
				f.Min = 100
				f.Max = 900
			}
			if strings.HasSuffix(field.key, "-alpha") {
				f.Max = 1
			}
			if field.key == "content" {
				for _, v := range []string{"danmaku", "status", "combined"} {
					f.Options = append(f.Options, webui.Option{ID: v, Label: overlayContentLabel(v)})
				}
			}
			if field.key == "anchor" {
				for _, v := range []string{"top-left", "top", "top-right", "left", "center", "right", "bottom-left", "bottom", "bottom-right"} {
					f.Options = append(f.Options, webui.Option{ID: v, Label: overlayAnchorLabel(v)})
				}
			}
		}
		f := add("tts-voice", "tts", i18n.OutputVoice, "select", m.config.TTS.Voice, false)
		for _, v := range tts.Voices() {
			f.Options = append(f.Options, webui.Option{ID: v.ID, Label: i18n.T(v.Name)})
		}
		f = add("tts-volume", "tts", i18n.OutputVolume, "range", strconv.Itoa(m.config.TTS.Volume), false)
		f.Max, f.Step = 100, 1
		for _, spec := range presentation.Specs() {
			add("overlay-event:"+spec.ID, "overlay-events", spec.Label, "switch", boolean(!slices.Contains(m.config.OverlayDisabledEvents, spec.ID)), false)
			add("tts-event:"+spec.ID, "tts-events", spec.Label, "switch", boolean(!slices.Contains(m.config.TTS.DisabledEvents, spec.ID)), false)
		}
	}
	return fields
}

func (m *Model) webSetField(page, id, value string) (tea.Cmd, error) {
	var field *webui.Field
	for _, f := range m.webFields(page) {
		if f.ID == id {
			field = &f
			break
		}
	}
	if field == nil || field.Disabled {
		return nil, errors.New(i18n.T(i18n.WebActionUnavailable))
	}
	if field.Kind == "switch" && value != "true" && value != "false" {
		return nil, errors.New(i18n.T(i18n.WebRequestInvalid))
	}
	if field.Kind == "select" {
		found := false
		for _, option := range field.Options {
			if option.ID == value && !option.Disabled {
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New(i18n.T(i18n.WebRequestInvalid))
		}
	}
	if field.Kind == "switch" && field.Value == value {
		return nil, nil
	}
	if strings.HasPrefix(id, "overlay-event:") || strings.HasPrefix(id, "tts-event:") {
		previous := m.page
		m.page = overlayPage
		if strings.HasPrefix(id, "tts-event:") {
			m.page = ttsPage
		}
		cmd := m.toggleOutputEvent(strings.SplitN(id, ":", 2)[1])
		m.page = previous
		return cmd, nil
	}
	if strings.HasPrefix(id, "overlay-") {
		if id == "overlay-enabled" {
			return m.performOverlay("overlay-toggle"), nil
		}
		if id == "overlay-preview" {
			return m.performOverlay("overlay-preview"), nil
		}
		s := m.config.Overlay
		switch id {
		case "overlay-backend":
			s.Backend = overlay.Backend(value)
		case "overlay-qt-align":
			s.Qt.Alignment = value
		case "overlay-qt-gradient":
			s.Qt.Gradient = value == "true"
		case "overlay-qt-animation":
			s.Qt.AnimationEnabled = value == "true"
		case "overlay-content":
			s.Content = value
		case "overlay-anchor":
			s.Position.Anchor = overlay.Anchor(value)
		case "overlay-italic":
			s.Font.Italic = value == "true"
		case "overlay-outline":
			s.Outline = value == "true"
		default:
			for _, f := range overlayFields {
				if id == "overlay-"+f.key && f.assign != nil {
					if err := f.assign(&s, value); err != nil {
						return nil, err
					}
					break
				}
			}
		}
		if _, err := s.Normalize(); err != nil {
			return nil, err
		}
		return m.saveOverlay(s, overlayConfigOperation()), nil
	}
	switch id {
	case "title":
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New(i18n.T(i18n.WebRequestInvalid))
		}
		return m.setTitle(value), nil
	case "chat-enabled":
		return m.perform("chat-toggle"), nil
	case "tts-enabled":
		return m.performTTS("tts-toggle"), nil
	case "obs-auto-connect", "obs-auto-stream", "chat-other", "exit-obs-stop", "exit-live-stop":
		return m.perform(id), nil
	case "protocol", "tts-voice":
		cfg := m.config
		if id == "protocol" {
			cfg.Protocol = value
		} else {
			cfg.TTS.Voice = value
		}
		return m.saveConfig(cfg, false), nil
	default:
		// 临时借用表单解析器，不向浏览器暴露普通编辑弹层。
		previousKind := m.editKind
		m.editKind = id
		cmd, err := m.webSubmit(value)
		m.editKind = previousKind
		return cmd, err
	}
}

func (m *Model) webSubmit(value string) (tea.Cmd, error) {
	if m.editKind == "clear-data" && value != "arcanaworldclear" {
		return nil, errors.New(i18n.T(i18n.TUISettingsClearDataMismatch))
	}
	previousMode := m.mode
	m.input.SetValue(value)
	cmd := m.submitForm()
	if cmd == nil && m.statusWarning && (!strings.HasPrefix(m.webTarget(), "chat-") || m.mode == previousMode) {
		m.mode = previousMode
		m.input.SetValue(value)
		return nil, errors.New(m.webText(m.status))
	}
	return cmd, nil
}

func (m *Model) webAction(page, action string) (tea.Cmd, error) {
	allowed := false
	switch page {
	case "studio":
		allowed = slices.Contains([]string{"start-confirm", "stop-confirm", "refresh", "areas", "cover", "delay", "obs-connect", "obs-disconnect-confirm"}, action)
	case "outputs":
		if action == "overlay-restore-confirm" {
			return m.confirm(i18n.T(i18n.TUIOverlayRestoreConfirm), "overlay-restore"), nil
		}
		allowed = slices.Contains([]string{"overlay-displays", "tts-preview", "tts-stop"}, action)
	case "settings":
		allowed = slices.Contains([]string{"obs-connect", "obs-disconnect-confirm", "settings-reset-confirm", "clear-data"}, action)
	case "accounts":
		allowed = action == "login"
		for _, a := range m.store.Accounts() {
			if action == "account:"+a.UID {
				return m.confirm(i18n.T(i18n.TUIConfirmSwitchAccount), "switch:"+a.UID), nil
			}
			if action == "delete:"+a.UID {
				prompt := fmt.Sprintf(i18n.T(i18n.TUIConfirmRemoveAccount), m.webText(a.Name+" / "+a.UID))
				if m.config.ActiveUID == a.UID {
					prompt += "\n" + i18n.T(i18n.TUIConfirmDetachAccount)
				}
				return m.confirm(prompt, action), nil
			}
		}
	}
	if !allowed {
		return nil, errors.New(i18n.T(i18n.WebActionUnavailable))
	}
	return m.perform(action), nil
}

func (m *Model) webTarget() string {
	if m.members != nil {
		return "chat-members"
	}
	if m.speaker != nil {
		if m.mode == "speaker" {
			return "chat-speaker"
		}
		return "chat-moderation"
	}
	if m.moderation != nil {
		return "chat-moderation"
	}
	switch m.mode {
	case "qr", "face", "login-save":
		return "account-login"
	case "selection":
		return "room-area"
	case "cover", "cover-review":
		return "room-cover"
	case "web-stream":
		return "stream-credentials"
	}
	if m.mode == "form" && m.editKind == "delay" {
		return "room-delay"
	}
	if m.mode == "pick" && m.editKind == "overlay-displays" {
		return "overlay-displays"
	}
	if m.mode == "confirm" {
		if strings.HasPrefix(m.confirmAction, "switch:") || strings.HasPrefix(m.confirmAction, "delete:") || m.confirmAction == "login-switch" {
			return "account-login"
		}
		if m.confirmAction == "overlay-restore" {
			return "overlay-displays"
		}
		if m.confirmAction == "start" || m.confirmAction == "stop" || m.confirmAction == "obs-disconnect" {
			return "room-session"
		}
	}
	return "maintenance"
}
