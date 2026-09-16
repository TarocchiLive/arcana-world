package tui

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"arcana-world/internal/bili"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type menuItem struct{ label, action string }

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
	case settingsPage:
		return []menuItem{
			{i18n.T(i18n.TUIMenuSetProxy), "proxy"},
			{i18n.T(i18n.TUIMenuSetProtocol), "protocol"},
			{toggleLabel(i18n.T(i18n.TUISettingsExitOBSStop), !m.config.ExitOBSStopDisabled), "exit-obs-stop"},
			{toggleLabel(i18n.T(i18n.TUISettingsExitLiveStop), !m.config.ExitLiveStopDisabled), "exit-live-stop"},
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
	if strings.HasPrefix(action, "account:") {
		return m.loadAccount(strings.TrimPrefix(action, "account:"))
	}
	if strings.HasPrefix(action, "delete:") {
		uid := strings.TrimPrefix(action, "delete:")
		return m.work("delete", func(ctx context.Context) (any, error) {
			if err := m.acquireOBS(ctx); err != nil {
				return nil, err
			}
			defer func() { <-m.obsLifecycle }()
			return uid, m.store.Delete(uid)
		})
	}
	switch action {
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
		return m.work("qr", func(ctx context.Context) (any, error) { return m.client.GenerateQR(ctx) })
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
		return m.work("areas", func(ctx context.Context) (any, error) {
			all, err := m.client.Areas(ctx)
			if err != nil {
				return nil, err
			}
			recent, historyErr := m.client.RecentAreas(ctx, roomID, all)
			return areaCatalog{all: all, recent: append(recent, local...), historyErr: historyErr}, nil
		})
	case "delay":
		if !m.requireRoom() {
			return nil
		}
		return m.work("delay", func(ctx context.Context) (any, error) { return m.client.TimeShift(ctx) })
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
	return m.work("refresh", func(ctx context.Context) (any, error) { return m.client.Room(ctx) })
}
func (m *Model) loadAccount(uid string) tea.Cmd {
	proxy := m.config.Proxy
	return m.work("account", func(ctx context.Context) (any, error) {
		if err := m.acquireOBS(ctx); err != nil {
			return nil, err
		}
		defer func() { <-m.obsLifecycle }()
		a, err := m.store.Load(uid)
		if err != nil {
			return nil, err
		}
		c, err := bili.New(proxy)
		if err != nil {
			return nil, err
		}
		c.SetAccount(a)
		info, err := c.Validate(ctx)
		if err != nil {
			return nil, err
		}
		a.Name = info.Name
		if err = m.store.Save(a); err != nil {
			return nil, err
		}
		cfg := m.store.Config()
		cfg.ActiveUID = a.UID
		if err = m.store.SaveConfig(cfg); err != nil {
			return nil, err
		}
		return accountResult{a, c}, nil
	})
}
func (m *Model) saveLogin(a domain.Account) tea.Cmd {
	proxy := m.config.Proxy
	return m.work("login-save", func(ctx context.Context) (any, error) {
		if err := m.acquireOBS(ctx); err != nil {
			return nil, err
		}
		defer func() { <-m.obsLifecycle }()
		c, err := bili.New(proxy)
		if err != nil {
			return nil, err
		}
		c.SetAccount(a)
		info, err := c.Validate(ctx)
		if err != nil {
			return nil, err
		}
		a.UID = info.UID
		a.Name = info.Name
		if err = m.store.Save(a); err != nil {
			return nil, err
		}
		cfg := m.store.Config()
		cfg.ActiveUID = a.UID
		if err = m.store.SaveConfig(cfg); err != nil {
			return nil, err
		}
		return accountResult{a, c}, nil
	})
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
	return nil
}
func (m *Model) choose() tea.Cmd {
	if m.selected < 0 || m.selected >= len(m.choices) {
		return nil
	}
	ch := m.choices[m.selected]
	m.mode = ""
	switch m.editKind {
	case "delete":
		return m.confirm(fmt.Sprintf(i18n.T(i18n.TUIConfirmRemoveAccount), ch.label), "delete:"+ch.value)
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
	return m.work("area", func(ctx context.Context) (any, error) {
		if err := m.client.SetArea(ctx, room, a.ID); err != nil {
			return nil, err
		}
		recent := []domain.Area{a}
		for _, old := range cfg.RecentAreas {
			if old.ID != a.ID && len(recent) < 10 {
				recent = append(recent, old)
			}
		}
		cfg.RecentAreas = recent
		return editResult{kind: "area", area: &a}, m.store.SaveConfig(cfg)
	})
}
func (m *Model) submitForm() tea.Cmd {
	value := strings.TrimSpace(m.input.Value())
	kind := m.editKind
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
		if _, err := bili.New(value); err != nil {
			m.status = clean(err.Error())
			return nil
		}
	}
	m.mode = ""
	m.input.SetValue("")
	m.input.Blur()
	switch kind {
	case "announcement":
		room := m.room.ID
		return m.work("announcement", func(ctx context.Context) (any, error) {
			if err := m.client.SetAnnouncement(ctx, room, value); err != nil {
				return nil, err
			}
			return editResult{kind: "announcement", value: value}, nil
		})
	case "cover-path":
		return m.prepareCover(value)
	case "delay":
		n, _ := strconv.Atoi(value)
		return m.work("delay-set", func(ctx context.Context) (any, error) { return nil, m.client.SetTimeShift(ctx, n) })
	case "obs-password":
		return m.work("obs-password", func(ctx context.Context) (any, error) {
			if err := m.acquireOBS(ctx); err != nil {
				return nil, err
			}
			defer func() { <-m.obsLifecycle }()
			if err := m.store.SetOBSSecret(value); err != nil {
				return nil, err
			}
			return nil, m.obsClient.Disconnect()
		})
	case "obs-url":
		cfg := m.config
		cfg.OBSURL = value
		return m.work("obs-url", func(ctx context.Context) (any, error) {
			if err := m.acquireOBS(ctx); err != nil {
				return nil, err
			}
			defer func() { <-m.obsLifecycle }()
			if err := m.store.SaveConfig(cfg); err != nil {
				return nil, err
			}
			return nil, m.obsClient.Disconnect()
		})
	case "proxy":
		cfg := m.config
		cfg.Proxy = value
		return m.saveConfig(cfg, true)
	}
	return nil
}
func (m *Model) saveConfig(cfg domain.Config, replaceClient bool) tea.Cmd {
	account := m.account
	return m.work("config", func(ctx context.Context) (any, error) {
		if err := m.acquireOBS(ctx); err != nil {
			return nil, err
		}
		defer func() { <-m.obsLifecycle }()
		var c *bili.Client
		if replaceClient {
			var err error
			c, err = bili.New(cfg.Proxy)
			if err != nil {
				return nil, err
			}
			if account != nil {
				c.SetAccount(*account)
			}
		}
		if err := m.store.SaveConfig(cfg); err != nil {
			return nil, err
		}
		if replaceClient {
			return c, nil
		}
		return nil, nil
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
	return m.work("title", func(ctx context.Context) (any, error) {
		if err := m.client.SetTitle(ctx, room, value); err != nil {
			return nil, err
		}
		titles := []string{value}
		for _, s := range cfg.RecentTitles {
			if s != value && len(titles) < 5 {
				titles = append(titles, s)
			}
		}
		cfg.RecentTitles = titles
		return editResult{kind: "title", value: value}, m.store.SaveConfig(cfg)
	})
}
