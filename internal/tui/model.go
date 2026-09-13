package tui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
	"sync"
	"time"
	"unicode"

	"arcana-world/internal/bili"
	"arcana-world/internal/coverimage"
	"arcana-world/internal/domain"
	"arcana-world/internal/journal"
	"arcana-world/internal/obs"
	"arcana-world/internal/store"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

var pages = []string{"直播", "账号", "直播间", "OBS", "设置", "日志", "帮助"}

type resultMsg struct {
	id    int
	kind  string
	value any
	err   error
}
type pollTick struct{ generation int }
type choice struct{ label, value string }
type accountResult struct {
	account domain.Account
	client  *bili.Client
}
type editResult struct {
	kind, value string
	area        *domain.Area
}

type Model struct {
	ctx                    context.Context
	store                  *store.Store
	client                 *bili.Client
	config                 domain.Config
	account                *domain.Account
	room                   *domain.Room
	stream                 *domain.Stream
	obsClient              *obs.Client
	obsState               obs.Snapshot
	obsBusy                bool
	obsCancel              context.CancelFunc
	obsOperation           int
	initialized            bool
	selection              *roomSelection
	cover                  *coverimage.Prepared
	previewing             bool
	page                   int
	cursors                [7]int
	width, height          int
	mode, editKind, prompt string
	input                  textinput.Model
	choices                []choice
	selected               int
	confirmAction          string
	busy                   bool
	canceled               bool
	cancel                 context.CancelFunc
	operation              int
	qrGeneration           int
	qr                     *domain.QR
	qrText                 string
	faceURL                string
	pendingAccount         *domain.Account
	status                 string
	logs                   []string
	journal                *journal.Log
	closeOnce              sync.Once
	closeErr               error
	reveal                 bool
	view                   viewport.Model
}

func New(ctx context.Context, s *store.Store) (*Model, error) {
	cfg := s.Config()
	disk, err := journal.Open(s.Dir())
	if err != nil {
		return nil, err
	}
	c, err := bili.New(cfg.Proxy)
	if err != nil {
		return nil, errors.Join(err, disk.Write("启动失败："+err.Error()), disk.Close())
	}
	if err := disk.Write("应用启动"); err != nil {
		return nil, errors.Join(err, disk.Close())
	}
	in := textinput.New()
	in.CharLimit = 4096
	m := &Model{ctx: ctx, store: s, config: cfg, client: c, journal: disk, obsClient: obs.NewClient(), width: 100, height: 32, input: in, status: "就绪 · 登录后可管理直播间", view: viewport.New(96, 24)}
	return m, nil
}
func (m *Model) Init() tea.Cmd {
	if m.initialized {
		return nil
	}
	m.initialized = true
	cmds := []tea.Cmd{m.watchOBS()}
	if m.config.OBSAutoConnect {
		cmds = append(cmds, m.connectOBS())
	}
	if m.config.ActiveUID != "" {
		cmds = append(cmds, m.loadAccount(m.config.ActiveUID))
	}
	return tea.Batch(cmds...)
}
func (m *Model) work(kind string, fn func(context.Context) (any, error)) tea.Cmd {
	if m.busy {
		return nil
	}
	m.busy = true
	m.canceled = false
	m.operation++
	id := m.operation
	ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
	m.cancel = cancel
	m.status = "正在" + operationName(kind) + "…"
	return func() tea.Msg { defer cancel(); value, err := fn(ctx); return resultMsg{id, kind, value, err} }
}
func operationName(kind string) string {
	switch kind {
	case "cover-prepare":
		return "处理封面"
	case "cover-fetch":
		return "读取封面"
	case "cover-upload":
		return "上传封面"
	}
	names := map[string]string{"refresh": "刷新直播间", "account": "加载账号", "qr": "生成二维码", "poll": "等待扫码", "login-save": "保存登录", "start": "开播", "stop": "关播", "areas": "获取分区", "face": "获取身份验证链接", "obs-connect": "连接 OBS", "obs-disconnect": "断开 OBS", "config": "保存设置", "delete": "删除账号", "delay": "读取直播延时"}
	if s, ok := names[kind]; ok {
		return s
	}
	return "更新" + kind
}
func (m *Model) log(s string) {
	s = m.safe(s)
	if err := m.journal.Write(s); err != nil {
		s += "（日志写入失败：" + m.safe(err.Error()) + "）"
	}
	m.logs = append(m.logs, time.Now().Format("15:04:05")+"  "+s)
	if len(m.logs) > 200 {
		m.logs = append([]string(nil), m.logs[len(m.logs)-200:]...)
	}
	m.status = s
}
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			if r == '\n' {
				return r
			}
			return -1
		}
		return r
	}, s)
}
func (m *Model) safe(s string) string {
	if m.account != nil {
		for _, v := range m.account.Cookies {
			if v != "" {
				s = strings.ReplaceAll(s, v, "[已隐藏]")
			}
		}
	}
	if m.stream != nil && m.stream.Key != "" {
		s = strings.ReplaceAll(s, m.stream.Key, "[已隐藏]")
	}
	return clean(s)
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case coverPreviewFinished:
		m.previewing = false
		if msg.err != nil {
			m.log("封面预览失败：" + msg.err.Error())
		} else {
			m.status = "预览已关闭"
		}
		return m, nil
	case obsEventMsg:
		return m, m.handleOBSEvent(msg)
	case obsResultMsg:
		return m, m.handleOBSResult(msg)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.view.Width = max(12, m.width-4)
		m.view.Height = max(3, m.height-10)
		m.input.Width = max(10, m.width-10)
	case resultMsg:
		if msg.id != m.operation {
			return m, nil
		}
		m.busy = false
		m.cancel = nil
		return m, m.result(msg)
	case pollTick:
		if msg.generation == m.qrGeneration && m.mode == "qr" && !m.busy && m.qr != nil {
			key := m.qr.Key
			return m, m.work("poll", func(ctx context.Context) (any, error) { return m.client.PollQR(ctx, key) })
		}
	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" || (key == "q" && m.mode == "") {
			if m.cancel != nil {
				m.cancel()
			}
			if m.obsCancel != nil {
				m.obsCancel()
			}
			return m, tea.Quit
		}
		if m.previewing {
			return m, nil
		}
		if m.busy {
			if key == "esc" && m.cancel != nil {
				m.canceled = true
				m.cancel()
				m.qrGeneration++
				m.mode = ""
				m.qr = nil
				m.qrText = ""
				m.faceURL = ""
				m.status = "已请求取消；已发送的服务端操作可能已生效，请刷新确认"
			}
			return m, nil
		}
		if m.obsBusy && key == "esc" && m.mode == "" {
			if m.obsCancel != nil {
				m.obsCancel()
			}
			_ = m.obsClient.Disconnect()
			m.status = "已取消 OBS 操作"
			return m, nil
		}
		if m.mode != "" {
			return m, m.modalKey(msg)
		}
		switch key {
		case "tab", "right":
			m.page = (m.page + 1) % len(pages)
			m.view.GotoTop()
		case "shift+tab", "left":
			m.page = (m.page + len(pages) - 1) % len(pages)
			m.view.GotoTop()
		case "1", "2", "3", "4", "5", "6", "7":
			m.page = int(key[0] - '1')
			m.view.GotoTop()
		case "up", "k":
			if m.cursors[m.page] > 0 {
				m.cursors[m.page]--
			}
		case "down", "j":
			if m.cursors[m.page]+1 < len(m.menu()) {
				m.cursors[m.page]++
			}
		case "enter":
			return m, m.activate()
		case "r":
			if m.account != nil {
				return m, m.refresh()
			}
		}
	}
	if m.mode == "selection" && m.selection != nil {
		return m, m.updateSelection(msg)
	}
	if m.mode != "form" {
		m.view.SetContent(m.content())
		var cmd tea.Cmd
		m.view, cmd = m.view.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
func (m *Model) modalKey(msg tea.KeyMsg) tea.Cmd {
	if m.mode == "cover" || m.mode == "cover-review" {
		return m.coverKey(msg)
	}
	if m.mode == "form" && m.editKind == "cover-path" && msg.String() == "esc" {
		m.mode = "cover"
		m.input.SetValue("")
		m.input.Blur()
		m.view.GotoTop()
		return nil
	}
	if m.mode == "selection" && m.selection != nil {
		return m.updateSelection(msg)
	}
	key := msg.String()
	if key == "esc" {
		m.mode = ""
		m.qrGeneration++
		m.qr = nil
		m.qrText = ""
		m.faceURL = ""
		m.pendingAccount = nil
		m.input.SetValue("")
		m.input.Blur()
		m.view.GotoTop()
		return nil
	}
	switch m.mode {
	case "form":
		if key == "enter" {
			return m.submitForm()
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return cmd
	case "pick":
		switch key {
		case "up", "k":
			m.selected = max(0, m.selected-1)
		case "down", "j":
			m.selected = min(len(m.choices)-1, m.selected+1)
		case "pgup":
			m.selected = max(0, m.selected-max(3, m.view.Height-5))
		case "pgdown":
			m.selected = min(len(m.choices)-1, m.selected+max(3, m.view.Height-5))
		case "home":
			m.selected = 0
		case "end":
			m.selected = len(m.choices) - 1
		case "enter":
			return m.choose()
		}
	case "confirm":
		switch key {
		case "left", "right", "tab", "h", "l":
			m.selected = 1 - m.selected
		case "enter":
			action := m.confirmAction
			m.mode = ""
			if m.selected == 1 {
				return m.perform(action)
			}
		}
	case "login-save":
		if key == "enter" && m.pendingAccount != nil {
			return m.saveLogin(*m.pendingAccount)
		}
	case "qr", "face":
		if key == "enter" {
			m.mode = ""
			m.qrGeneration++
			m.qr = nil
			m.qrText = ""
			m.faceURL = ""
		}
	}
	m.view.SetContent(m.content())
	m.view, _ = m.view.Update(msg)
	return nil
}
func (m *Model) result(r resultMsg) tea.Cmd {
	if m.canceled {
		switch r.kind {
		case "qr", "poll", "face", "areas", "delay", "refresh", "cover-prepare", "cover-fetch":
			m.log("操作已取消")
			return nil
		}
	}
	// Apply any confirmed remote mutation even when subsequent local history saving failed.
	if edit, ok := r.value.(editResult); ok && m.room != nil {
		switch edit.kind {
		case "title":
			m.room.Title = edit.value
		case "announcement":
			m.room.Announcement = edit.value
		case "area":
			m.room.AreaID = edit.area.ID
			m.room.AreaName = edit.area.Name
			m.room.ParentName = edit.area.Parent
		}
	}
	if r.err != nil {
		var face *domain.FaceChallenge
		if errors.As(r.err, &face) && !m.canceled {
			return m.work("face", func(ctx context.Context) (any, error) { return m.client.ResolveFace(ctx, face) })
		}
		if errors.Is(r.err, context.Canceled) {
			m.log("操作已取消；网络写入可能已生效，请刷新确认")
		} else {
			m.log(operationName(r.kind) + "失败：" + r.err.Error())
		}
		if r.kind == "poll" {
			m.mode = ""
			m.qr = nil
			m.qrText = ""
			m.qrGeneration++
		}
		if r.kind == "login-save" {
			if m.canceled {
				m.pendingAccount = nil
			} else {
				m.mode = "login-save"
			}
		}
		if r.kind == "cover-prepare" || r.kind == "cover-fetch" {
			m.mode = "cover"
		}
		return nil
	}
	m.config = m.store.Config()
	switch r.kind {
	case "cover-prepare":
		m.cover = r.value.(*coverimage.Prepared)
		m.mode = "cover-review"
		m.view.GotoTop()
		m.status = "封面已处理；p 预览，Enter 确认上传"
	case "cover-fetch":
		return m.previewCover(r.value.(image.Image), "当前直播封面")
	case "cover-upload":
		update := r.value.(bili.CoverUpdate)
		m.room.CoverURL = update.URL
		m.room.CoverStatus = update.Status
		if update.Reason != "" {
			m.room.CoverStatus += " · " + m.safe(update.Reason)
		}
		m.cover = nil
		m.mode = "cover"
		m.log("封面已提交：" + m.room.CoverStatus)
	case "account", "login-save":
		a := r.value.(accountResult)
		m.account = &a.account
		m.client = a.client
		m.room = nil
		m.stream = nil
		m.reveal = false
		m.pendingAccount = nil
		m.mode = ""
		m.qr = nil
		m.qrText = ""
		m.qrGeneration++
		m.log("已切换账号：" + a.account.Name + " / " + a.account.UID)
		return m.refresh()
	case "refresh":
		room := r.value.(domain.Room)
		m.room = &room
		if !room.Live {
			m.stream = nil
			m.reveal = false
		}
		m.log("直播间状态已刷新")
	case "qr":
		qr := r.value.(domain.QR)
		m.qr = &qr
		m.mode = "qr"
		m.qrGeneration++
		m.qrText = renderQR(qr.URL)
		m.view.GotoTop()
		m.status = "请用哔哩哔哩手机 App 扫码登录"
		return m.nextPoll()
	case "poll":
		if m.mode != "qr" {
			return nil
		}
		poll := r.value.(domain.LoginPoll)
		switch poll.Code {
		case 0:
			if poll.Account == nil {
				m.log("登录响应缺少凭据，请重新扫码")
				m.mode = ""
				return nil
			}
			m.pendingAccount = poll.Account
			m.qr = nil
			m.qrText = ""
			m.qrGeneration++
			return m.saveLogin(*poll.Account)
		case 86101:
			m.status = "等待扫码…"
		case 86090:
			m.status = "已扫描，请在手机上确认登录"
		case 86038:
			m.log("二维码已过期，请重新选择扫码登录")
			m.mode = ""
			m.qr = nil
			m.qrText = ""
			return nil
		default:
			m.log(fmt.Sprintf("登录状态异常：%d，请重新扫码", poll.Code))
			m.mode = ""
			return nil
		}
		return m.nextPoll()
	case "start":
		outcome := r.value.(startOutcome)
		s := outcome.stream
		m.stream = &s
		m.reveal = false
		if m.room != nil {
			m.room.Live = true
		}
		m.log("B 站已开播")
		if outcome.obsStarted {
			m.log("B 站已开播，OBS 已同步推流")
		} else if outcome.obsConfigured {
			m.log("B 站已开播，推流配置已写入 OBS；自动推流未开启")
		}
		if outcome.obsErr != nil {
			m.log("B 站已开播，但 OBS 联动失败：" + outcome.obsErr.Error())
		}
		if s.Warning != "" {
			m.log(s.Warning)
		}
	case "stop":
		if m.room != nil {
			m.room.Live = false
		}
		m.stream = nil
		m.reveal = false
		m.log("B 站已关播")
		if outcome, ok := r.value.(stopOutcome); ok {
			if outcome.obsStopped {
				m.log("B 站已关播，OBS 已停止推流")
			}
			if outcome.warning != "" {
				m.log("B 站已关播；" + outcome.warning)
			}
		}
	case "face":
		m.faceURL = r.value.(string)
		m.qrText = renderQR(m.faceURL)
		m.mode = "face"
		m.view.GotoTop()
		m.status = "请完成 B 站要求的身份验证，再返回重新开播"
	case "areas":
		catalog := r.value.(areaCatalog)
		m.selection = newAreaSelection(catalog.all, catalog.recent, m.room.AreaID)
		m.mode = "selection"
		m.view.GotoTop()
		m.status = "选择最近分区，或进入父分类 / 输入名称和编号搜索"
		if catalog.historyErr != nil {
			m.log("最近分区获取失败，仍可使用本地最近记录和完整分类：" + catalog.historyErr.Error())
		}
		return m.selection.Init()
	case "delay":
		return m.form("delay", "直播延时（秒）", fmt.Sprint(r.value.(int)), false)
	case "delete":
		if m.account != nil && m.account.UID == r.value.(string) {
			m.account = nil
			m.room = nil
			m.stream = nil
			m.reveal = false
			m.client, _ = bili.New(m.config.Proxy)
		}
		m.log("账号已从本应用移除，不影响其他客户端")
	case "config":
		if c, ok := r.value.(*bili.Client); ok {
			m.client = c
		}
		m.log("设置已保存")
	default:
		m.log(operationName(r.kind) + "成功")
	}
	m.view.SetContent(m.content())
	return nil
}
func (m *Model) nextPoll() tea.Cmd {
	id := m.qrGeneration
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return pollTick{id} })
}
