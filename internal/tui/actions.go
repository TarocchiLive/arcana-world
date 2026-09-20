package tui

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"arcana-world/internal/app"
	"arcana-world/internal/bili"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type menuItem struct{ label, action string }

const (
	recentAreaLimit  = 10
	recentTitleLimit = 5
)

func (m *Model) menu() []menuItem {
	switch m.page {
	case livePage:
		liveAction := menuItem{i18n.T(i18n.TUIMenuStartLive), "start-confirm"}
		if m.room != nil && m.room.Live {
			liveAction = menuItem{i18n.T(i18n.TUIMenuStopLive), "stop-confirm"}
		}
		return []menuItem{{i18n.T(i18n.TUIMenuRefreshRoom), "refresh"}, liveAction, {i18n.T(i18n.TUIMenuRevealStreamKey), "reveal"}}
	case accountsPage:
		items := []menuItem{{i18n.T(i18n.TUIMenuAddAccount), "login"}}
		for _, a := range m.store.Accounts() {
			items = append(items, menuItem{clean(a.Name) + " · " + a.UID, "account:" + a.UID})
		}
		return append(items, menuItem{i18n.T(i18n.TUIMenuRemoveAccount), "delete-pick"})
	case roomPage:
		return []menuItem{{i18n.T(i18n.TUIMenuEditTitle), "title"}, {i18n.T(i18n.TUIMenuSelectCategory), "areas"}, {i18n.T(i18n.TUIMenuEditAnnouncement), "announcement"}, {i18n.T(i18n.TUIMenuManageCover), "cover"}, {i18n.T(i18n.TUIMenuManageDelay), "delay"}}
	case obsPage:
		label := i18n.T(i18n.TUIOBSConnect)
		action := "obs-connect"
		if m.obsBusy || m.obsState.Connecting {
			label = i18n.T(i18n.TUIOBSConnecting)
		}
		if m.obsState.Connected {
			label = i18n.T(i18n.TUIOBSDisconnect)
			action = "obs-disconnect-confirm"
		}
		return []menuItem{
			{label, action},
			{toggleLabel(i18n.T(i18n.TUIOBSAutoConnect), m.config.OBSAutoConnect), "obs-auto-connect"},
			{toggleLabel(i18n.T(i18n.TUIOBSAutoStream), m.config.OBSAutoStream), "obs-auto-stream"},
			{i18n.T(i18n.TUISettingsOBSURL), "obs-url"},
			{i18n.T(i18n.TUIMenuOBSPassword), "obs-password"},
		}
	case overlayPage, ttsPage:
		return m.outputMenu()
	case settingsPage:
		return []menuItem{
			{i18n.T(i18n.TUIMenuSetProxy), "proxy"},
			{i18n.T(i18n.TUIMenuSetProtocol), "protocol"},
			{toggleLabel(i18n.T(i18n.TUISettingsExitOBSStop), !m.config.ExitOBSStopDisabled), "exit-obs-stop"},
			{toggleLabel(i18n.T(i18n.TUISettingsExitLiveStop), !m.config.ExitLiveStopDisabled), "exit-live-stop"},
			{i18n.T(i18n.TUISettingsReset), "settings-reset-confirm"},
			{i18n.T(i18n.TUISettingsClearData), "clear-data"},
		}
	case chatPage:
		return []menuItem{{toggleLabel(i18n.T(i18n.DanmakuToggle), !m.config.DanmakuDisabled), "chat-toggle"}}
	}
	return nil
}
func (m *Model) activate() tea.Cmd {
	items := m.menu()
	if len(items) == 0 {
		return nil
	}
	m.cursors[m.page] = min(m.cursors[m.page], len(items)-1)
	return m.perform(items[m.cursors[m.page]].action)
}
func (m *Model) requireRoom() bool {
	if m.account == nil {
		m.log(i18n.T(i18n.TUIStatusAccountRequired))
		return false
	}
	if m.room == nil {
		m.log(i18n.T(i18n.TUIStatusRoomRequired))
		return false
	}
	return true
}
func (m *Model) perform(action string) tea.Cmd {
	if m.busy {
		return nil
	}
	if action == "output-events" {
		return m.pickOutputEvents()
	}
	if action == "tts-voice" {
		return m.pickTTSVoice()
	}
	if strings.HasPrefix(action, "tts-") {
		return m.performTTS(action)
	}
	if strings.HasPrefix(action, "overlay-") {
		return m.performOverlay(action)
	}
	if strings.HasPrefix(action, "account:") {
		uid := strings.TrimPrefix(action, "account:")
		if m.config.ActiveUID != "" && m.config.ActiveUID != uid {
			return m.confirm(i18n.T(i18n.TUIConfirmSwitchAccount), "switch:"+uid)
		}
		return m.loadAccount(uid)
	}
	if strings.HasPrefix(action, "switch:") {
		return m.loadAccount(strings.TrimPrefix(action, "switch:"))
	}
	if action == "login-switch" && m.pendingAccount != nil {
		return m.commitLogin(*m.pendingAccount)
	}
	if strings.HasPrefix(action, "delete:") {
		uid, client := strings.TrimPrefix(action, "delete:"), m.client
		return work(m, deleteOperation(), func(ctx context.Context) (app.AccountOutcome, error) { return m.session.Delete(ctx, client, uid) })
	}
	switch action {
	case "settings-reset-confirm":
		return m.confirm(i18n.T(i18n.TUISettingsResetConfirm), "settings-reset")
	case "settings-reset":
		return m.resetSettings()
	case "clear-data":
		return m.form("clear-data", i18n.T(i18n.TUISettingsClearDataConfirm), "", false)
	case "exit-obs-stop":
		cfg := m.config
		cfg.ExitOBSStopDisabled = !cfg.ExitOBSStopDisabled
		return m.saveConfig(cfg, false)
	case "exit-live-stop":
		cfg := m.config
		cfg.ExitLiveStopDisabled = !cfg.ExitLiveStopDisabled
		return m.saveConfig(cfg, false)
	case "chat-toggle":
		if !m.config.DanmakuDisabled {
			return m.confirm(i18n.T(i18n.DanmakuToggleConfirm), "chat-disable")
		}
		cfg := m.config
		cfg.DanmakuDisabled = false
		return m.saveConfig(cfg, false)
	case "chat-disable":
		cfg := m.config
		cfg.DanmakuDisabled = true
		return m.saveConfig(cfg, false)
	case "login":
		return work(m, qrOperation(), func(ctx context.Context) (domain.QR, error) { return m.client.GenerateQR(ctx) })
	case "refresh":
		if m.account == nil {
			m.log(i18n.T(i18n.TUIStatusSignInRequired))
			return nil
		}
		return m.refresh()
	case "start-confirm":
		if !m.requireRoom() {
			return nil
		}
		if m.room.AreaID == 0 {
			m.log(i18n.T(i18n.TUIStatusCategoryRequired))
			return nil
		}
		if m.obsBusy {
			m.log(i18n.T(i18n.TUIStatusWaitOBSConnection))
			return nil
		}
		return m.confirm(m.startPrompt(), "start")
	case "stop-confirm":
		if !m.requireRoom() {
			return nil
		}
		if m.obsBusy {
			m.log(i18n.T(i18n.TUIStatusWaitOBSConnection))
			return nil
		}
		return m.confirm(m.stopPrompt(), "stop")
	case "start":
		return m.startLive()
	case "stop":
		return m.stopLive()
	case "reveal":
		if m.stream == nil {
			m.log(i18n.T(i18n.TUIStatusStreamUnavailable))
		} else {
			m.reveal = !m.reveal
		}
		return nil
	case "title":
		if !m.requireRoom() {
			return nil
		}
		m.selection = newTitleSelection(m.room.Title, m.store.Config().RecentTitles)
		m.mode = "selection"
		m.status = i18n.T(i18n.TUIStatusTitleEditHint)
		m.view.GotoTop()
		return m.selection.Init()
	case "announcement":
		if !m.requireRoom() {
			return nil
		}
		return m.form("announcement", i18n.T(i18n.TUIFormAnnouncement), m.room.Announcement, false)
	case "cover":
		if !m.requireRoom() {
			return nil
		}
		m.cover = nil
		m.mode = "cover"
		m.status = i18n.T(i18n.TUIStatusCoverActions)
		m.view.GotoTop()
		return nil
	case "areas":
		if !m.requireRoom() {
			return nil
		}
		roomID := m.room.ID
		local := m.store.Config().RecentAreas
		return work(m, areasOperation(), func(ctx context.Context) (areaCatalog, error) {
			all, err := m.client.Areas(ctx)
			if err != nil {
				return areaCatalog{}, err
			}
			recent, historyErr := m.client.RecentAreas(ctx, roomID, all)
			return areaCatalog{all: all, recent: append(recent, local...), historyErr: historyErr}, nil
		})
	case "delay":
		if !m.requireRoom() {
			return nil
		}
		return work(m, delayOperation(), func(ctx context.Context) (int, error) { return m.client.TimeShift(ctx) })
	case "delete-pick":
		m.choices = nil
		for _, a := range m.store.Accounts() {
			m.choices = append(m.choices, choice{clean(a.Name) + " / " + a.UID, a.UID})
		}
		return m.pick("delete", i18n.T(i18n.TUIFormRemoveAccount))
	case "obs-url":
		if m.obsBusy {
			m.log(i18n.T(i18n.TUIStatusWaitOBSSettings))
			return nil
		}
		return m.form("obs-url", i18n.T(i18n.TUISettingsOBSURL), m.config.OBSURL, false)
	case "obs-password":
		if m.obsBusy {
			m.log(i18n.T(i18n.TUIStatusWaitOBSSettings))
			return nil
		}
		return m.form("obs-password", i18n.T(i18n.TUIFormOBSPassword), "", true)
	case "proxy":
		if m.store.Overrides().Proxy != nil {
			m.log(i18n.T(i18n.TUISessionOverride))
			return nil
		}
		return m.form("proxy", i18n.T(i18n.TUIFormProxy), m.config.Proxy, false)
	case "protocol":
		m.choices = []choice{{"RTMP", "rtmp"}, {i18n.T(i18n.TUIProtocolSRTFallback), "srt-fallback"}, {i18n.T(i18n.TUIProtocolSRTOnly), "srt"}}
		return m.pick("protocol", i18n.T(i18n.TUIFormProtocol))
	case "obs-connect":
		return m.connectOBS()
	case "obs-disconnect-confirm":
		if m.obsBusy {
			return nil
		}
		return m.confirm(i18n.T(i18n.TUIConfirmDisconnectOBS), "obs-disconnect")
	case "obs-disconnect":
		return m.disconnectOBS()
	case "obs-auto-connect", "obs-auto-stream":
		overrides := m.store.Overrides()
		if action == "obs-auto-connect" && overrides.OBSAutoConnect != nil || action == "obs-auto-stream" && overrides.OBSAutoStream != nil {
			m.log(i18n.T(i18n.TUISessionOverride))
			return nil
		}
		if m.obsBusy {
			m.log(i18n.T(i18n.TUIStatusWaitOBS))
			return nil
		}
		cfg := m.config
		if action == "obs-auto-connect" {
			cfg.OBSAutoConnect = !cfg.OBSAutoConnect
		} else {
			cfg.OBSAutoStream = !cfg.OBSAutoStream
		}
		return m.saveConfig(cfg, false)
	}
	return nil
}
func (m *Model) refresh() tea.Cmd {
	return work(m, refreshOperation(), func(ctx context.Context) (domain.Room, error) { return m.client.Room(ctx) })
}
func (m *Model) loadAccount(uid string) tea.Cmd {
	client := m.client
	return work(m, accountOperation(), func(ctx context.Context) (app.AccountOutcome, error) { return m.session.Switch(ctx, client, uid, nil) })
}
func (m *Model) saveLogin(a domain.Account) tea.Cmd {
	uid := a.UID
	if uid == "" {
		uid = a.Cookies["DedeUserID"]
	}
	if m.config.ActiveUID != "" && m.config.ActiveUID != uid {
		m.pendingAccount = &a
		return m.confirm(i18n.T(i18n.TUIConfirmSwitchAccount), "login-switch")
	}
	return m.commitLogin(a)
}
func (m *Model) commitLogin(a domain.Account) tea.Cmd {
	client := m.client
	return work(m, loginSaveOperation(), func(ctx context.Context) (app.AccountOutcome, error) { return m.session.Switch(ctx, client, "", &a) })
}
func (m *Model) form(kind, prompt, value string, secret bool) tea.Cmd {
	m.mode = "form"
	m.editKind = kind
	m.prompt = prompt
	m.input.SetValue(value)
	m.input.EchoMode = textinput.EchoNormal
	if secret {
		m.input.EchoMode = textinput.EchoPassword
	}
	m.input.CursorEnd()
	m.view.GotoTop()
	return m.input.Focus()
}
func (m *Model) pick(kind, prompt string) tea.Cmd {
	if len(m.choices) == 0 {
		m.log(i18n.T(i18n.TUIStatusNoEntries))
		return nil
	}
	m.mode = "pick"
	m.editKind = kind
	m.prompt = prompt
	m.selected = 0
	m.view.GotoTop()
	return nil
}
func (m *Model) confirm(prompt, action string) tea.Cmd {
	m.mode = "confirm"
	m.prompt = prompt
	m.confirmAction = action
	m.selected = 0
	m.view.GotoTop()
	return nil
}
func (m *Model) choose() tea.Cmd {
	if m.selected < 0 || m.selected >= len(m.choices) {
		return nil
	}
	ch := m.choices[m.selected]
	if m.editKind == "output-events" {
		return m.toggleOutputEvent(ch.value)
	}
	m.mode = ""
	if strings.HasPrefix(m.editKind, "overlay-") {
		return m.chooseOverlay(ch.value)
	}
	switch m.editKind {
	case "tts-voice":
		cfg := m.config
		cfg.TTS.Voice = ch.value
		return m.saveConfig(cfg, false)
	case "delete":
		prompt := fmt.Sprintf(i18n.T(i18n.TUIConfirmRemoveAccount), ch.label)
		if m.config.ActiveUID == ch.value {
			prompt += "\n" + i18n.T(i18n.TUIConfirmDetachAccount)
		}
		return m.confirm(prompt, "delete:"+ch.value)
	case "protocol":
		cfg := m.config
		cfg.Protocol = ch.value
		return m.saveConfig(cfg, false)
	}
	return nil
}
func (m *Model) setArea(a domain.Area) tea.Cmd {
	room := m.room.ID
	cfg := m.store.Config()
	return work(m, areaOperation(), func(ctx context.Context) (*domain.Area, error) {
		if err := m.client.SetArea(ctx, room, a.ID); err != nil {
			return nil, err
		}
		cfg.RecentAreas = prependRecent(cfg.RecentAreas, a, recentAreaLimit, func(a domain.Area) int64 { return a.ID })
		return &a, m.store.SaveConfig(cfg)
	})
}
func (m *Model) submitForm() tea.Cmd {
	value := strings.TrimSpace(m.input.Value())
	kind := m.editKind
	if kind == "clear-data" {
		if m.input.Value() != "arcanaworldclear" {
			m.mode = ""
			m.input.SetValue("")
			m.input.Blur()
			m.view.GotoTop()
			m.log(i18n.T(i18n.TUISettingsClearDataMismatch))
			return nil
		}
		m.clearDataOnExit = true
		m.session.BeginClose()
		m.input.SetValue("")
		m.input.Blur()
		return tea.Quit
	}
	if strings.HasPrefix(kind, "overlay-") {
		return m.submitOverlay(kind, value)
	}
	// 密码保留原始字节，不去除首尾空白。
	if kind == "obs-password" {
		value = m.input.Value()
	}
	switch kind {
	case "cover-path":
		if value == "" {
			m.status = i18n.T(i18n.TUIValidationImagePath)
			return nil
		}
	case "delay":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			m.status = i18n.T(i18n.TUIValidationDelay)
			return nil
		}
	case "obs-url":
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			m.status = i18n.T(i18n.TUIValidationOBSURL)
			return nil
		}
	case "proxy":
		if m.store.Overrides().Proxy != nil {
			m.log(i18n.T(i18n.TUISessionOverride))
			return nil
		}
		client, err := bili.New(value)
		if err != nil {
			m.status = clean(err.Error())
			return nil
		}
		client.HTTP.CloseIdleConnections()
	}
	m.mode = ""
	m.input.SetValue("")
	m.input.Blur()
	switch kind {
	case "announcement":
		room := m.room.ID
		return work(m, announcementOperation(), func(ctx context.Context) (*string, error) {
			if err := m.client.SetAnnouncement(ctx, room, value); err != nil {
				return nil, err
			}
			return &value, nil
		})
	case "cover-path":
		return m.prepareCover(value)
	case "delay":
		n, _ := strconv.Atoi(value)
		return work(m, delaySetOperation, func(ctx context.Context) (struct{}, error) { return struct{}{}, m.client.SetTimeShift(ctx, n) })
	case "obs-password":
		return m.saveOBSSetting(obsPasswordOperation, func() error { return m.store.SetOBSSecret(value) })
	case "obs-url":
		cfg := m.config
		cfg.OBSURL = value
		return m.saveOBSSetting(obsURLOperation, func() error { return m.store.SaveConfig(cfg) })
	case "proxy":
		cfg := m.config
		cfg.Proxy = value
		return m.saveConfig(cfg, true)
	}
	return nil
}
func (m *Model) saveConfig(cfg domain.Config, replaceClient bool) tea.Cmd {
	if replaceClient {
		return m.rebuildClientAndSave(configOperation(), cfg.Proxy, func() error { return m.store.SaveConfig(cfg) })
	}
	return work(m, configOperation(), func(ctx context.Context) (*bili.Client, error) {
		if err := m.session.Lock(ctx); err != nil {
			return nil, err
		}
		defer m.session.Unlock()
		return nil, m.store.SaveConfig(cfg)
	})
}

type areaCatalog struct {
	all        []domain.Area
	recent     []domain.Area
	historyErr error
}

func (m *Model) updateSelection(msg tea.Msg) tea.Cmd {
	result, cmd := m.selection.Update(msg)
	if result == nil {
		return cmd
	}
	m.mode = ""
	m.selection = nil
	m.view.GotoTop()
	if result.Canceled {
		return nil
	}
	if result.Area != nil {
		return m.setArea(*result.Area)
	}
	return m.setTitle(result.Title)
}

func (m *Model) setTitle(value string) tea.Cmd {
	room := m.room.ID
	cfg := m.store.Config()
	return work(m, titleOperation(), func(ctx context.Context) (*string, error) {
		if err := m.client.SetTitle(ctx, room, value); err != nil {
			return nil, err
		}
		cfg.RecentTitles = prependRecent(cfg.RecentTitles, value, recentTitleLimit, func(s string) string { return s })
		return &value, m.store.SaveConfig(cfg)
	})
}

// prependRecent preserves all other entries, including their duplicates.
func prependRecent[T any, K comparable](history []T, value T, limit int, identity func(T) K) []T {
	if limit <= 0 {
		return nil
	}
	recent := make([]T, 1, min(limit, len(history)+1))
	recent[0] = value
	key := identity(value)
	for _, old := range history {
		if len(recent) == limit {
			break
		}
		if identity(old) != key {
			recent = append(recent, old)
		}
	}
	return recent
}
