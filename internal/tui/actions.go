package tui

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"arcana-world/internal/bili"
	"arcana-world/internal/domain"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type menuItem struct{ label, action string }

func (m *Model) menu() []menuItem {
	switch m.page {
	case 0:
		return []menuItem{{"刷新直播间状态", "refresh"}, {"开播 / 获取推流地址", "start-confirm"}, {"停止 B 站直播", "stop-confirm"}, {"显示 / 隐藏推流密钥", "reveal"}}
	case 1:
		items := []menuItem{{"扫码添加账号", "login"}}
		for _, a := range m.store.Accounts() {
			items = append(items, menuItem{clean(a.Name) + " · " + a.UID, "account:" + a.UID})
		}
		return append(items, menuItem{"移除已保存账号…", "delete-pick"})
	case 2:
		return []menuItem{{"标题（编辑 / 最近使用）", "title"}, {"分区（最近 / 分类 / 搜索）", "areas"}, {"修改公告", "announcement"}, {"封面（预览 / 上传）", "cover"}, {"读取 / 修改直播延时", "delay"}}
	case 3:
		label := "连接 OBS"
		action := "obs-connect"
		if m.obsBusy || m.obsState.Connecting {
			label = "正在连接 / 操作中…"
		}
		if m.obsState.Connected {
			label = "断开 OBS"
			action = "obs-disconnect-confirm"
		}
		return []menuItem{{label, action}, {toggleLabel("自动连接", m.config.OBSAutoConnect), "obs-auto-connect"}, {toggleLabel("自动推流", m.config.OBSAutoStream), "obs-auto-stream"}}
	case 4:
		return []menuItem{{"设置网络代理", "proxy"}, {"设置推流协议", "protocol"}, {"OBS WebSocket 地址", "obs-url"}, {"OBS 密码（系统密钥环）", "obs-password"}}
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
		m.log("请先在账号页扫码登录或选择账号")
		return false
	}
	if m.room == nil {
		m.log("请先刷新直播间状态")
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
		return m.work("delete", func(context.Context) (any, error) { return uid, m.store.Delete(uid) })
	}
	switch action {
	case "login":
		return m.work("qr", func(ctx context.Context) (any, error) { return m.client.GenerateQR(ctx) })
	case "refresh":
		if m.account == nil {
			m.log("请先登录账号")
			return nil
		}
		return m.refresh()
	case "start-confirm":
		if !m.requireRoom() {
			return nil
		}
		if m.room.AreaID == 0 {
			m.log("请先在直播间页选择分区")
			return nil
		}
		if m.obsBusy {
			m.log("请等待 OBS 连接操作完成")
			return nil
		}
		return m.confirm(m.startPrompt(), "start")
	case "stop-confirm":
		if !m.requireRoom() {
			return nil
		}
		if m.obsBusy {
			m.log("请等待 OBS 连接操作完成")
			return nil
		}
		return m.confirm(m.stopPrompt(), "stop")
	case "start":
		return m.startLive()
	case "stop":
		return m.stopLive()
	case "reveal":
		if m.stream == nil {
			m.log("开播成功后才能查看推流地址")
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
		m.status = "直接编辑标题，或选择最近记录；Tab 可填入后修改"
		m.view.GotoTop()
		return m.selection.Init()
	case "announcement":
		if !m.requireRoom() {
			return nil
		}
		return m.form("announcement", "直播间公告", m.room.Announcement, false)
	case "cover":
		if !m.requireRoom() {
			return nil
		}
		m.cover = nil
		m.mode = "cover"
		m.status = "p 预览当前封面，u 选择上传图片"
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
		return m.pick("delete", "选择要移除的账号")
	case "obs-url":
		if m.obsBusy {
			m.log("请等待 OBS 操作完成后修改连接设置")
			return nil
		}
		return m.form("obs-url", "OBS WebSocket 地址", m.config.OBSURL, false)
	case "obs-password":
		if m.obsBusy {
			m.log("请等待 OBS 操作完成后修改连接设置")
			return nil
		}
		return m.form("obs-password", "OBS WebSocket 密码（留空表示无密码）", "", true)
	case "proxy":
		return m.form("proxy", "代理：留空=系统环境，direct=直连，或 http(s):// / socks5://", m.config.Proxy, false)
	case "protocol":
		m.choices = []choice{{"RTMP", "rtmp"}, {"SRT 优先，无 SRT 时退回 RTMP", "srt-fallback"}, {"仅 SRT（不支持时报告错误）", "srt"}}
		return m.pick("protocol", "选择推流协议")
	case "obs-connect":
		return m.connectOBS()
	case "obs-disconnect-confirm":
		if m.obsBusy {
			return nil
		}
		return m.confirm("断开 OBS 控制连接？不会停止正在进行的推流。", "obs-disconnect")
	case "obs-disconnect":
		return m.disconnectOBS()
	case "obs-auto-connect", "obs-auto-stream":
		if m.obsBusy {
			m.log("请等待 OBS 操作完成")
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
		m.log("暂无可选记录")
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
		return m.confirm("从本应用删除账号 "+ch.label+" 的凭据？", "delete:"+ch.value)
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
	return m.work("分区", func(ctx context.Context) (any, error) {
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
	// Password bytes are not trimmed.
	if kind == "obs-password" {
		value = m.input.Value()
	}
	switch kind {
	case "cover-path":
		if value == "" {
			m.status = "请输入图片路径"
			return nil
		}
	case "delay":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			m.status = "延时必须为非负整数秒"
			return nil
		}
	case "obs-url":
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			m.status = "请输入不含凭据或查询参数的 ws:// 或 wss:// 地址"
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
		return m.work("公告", func(ctx context.Context) (any, error) {
			if err := m.client.SetAnnouncement(ctx, room, value); err != nil {
				return nil, err
			}
			return editResult{kind: "announcement", value: value}, nil
		})
	case "cover-path":
		return m.prepareCover(value)
	case "delay":
		n, _ := strconv.Atoi(value)
		return m.work("延时", func(ctx context.Context) (any, error) { return nil, m.client.SetTimeShift(ctx, n) })
	case "obs-password":
		return m.work("OBS 密码", func(context.Context) (any, error) {
			if err := m.store.SetOBSSecret(value); err != nil {
				return nil, err
			}
			return nil, m.obsClient.Disconnect()
		})
	case "obs-url":
		cfg := m.config
		cfg.OBSURL = value
		return m.work("OBS 地址", func(context.Context) (any, error) {
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
	return m.work("config", func(context.Context) (any, error) {
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
	return m.work("标题", func(ctx context.Context) (any, error) {
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
