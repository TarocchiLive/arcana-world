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

	"arcana-world/internal/app"
	"arcana-world/internal/bili"
	"arcana-world/internal/coverimage"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"arcana-world/internal/journal"
	"arcana-world/internal/obs"
	"arcana-world/internal/store"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	livePage = iota
	chatPage
	accountsPage
	roomPage
	obsPage
	settingsPage
	logsPage
	helpPage
	pageCount
)

func pageNames() [pageCount]string {
	return [pageCount]string{
		livePage:     i18n.T(i18n.TUIPageLive),
		chatPage:     i18n.T(i18n.DanmakuPage),
		accountsPage: i18n.T(i18n.TUIPageAccounts),
		roomPage:     i18n.T(i18n.TUIPageRoom),
		obsPage:      "OBS",
		settingsPage: i18n.T(i18n.TUIPageSettings),
		logsPage:     i18n.T(i18n.TUIPageLogs),
		helpPage:     i18n.T(i18n.TUIPageHelp),
	}
}

type resultMsg struct {
	id    int
	kind  string
	value any
	err   error
}
type pollTick struct{ generation int }
type choice struct{ label, value string }
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
	session                *app.Session
	initialized            bool
	overlay                *overlayRuntime
	overlayEnabled         bool
	overlayChat            overlayChatState
	selection              *roomSelection
	cover                  *coverimage.Prepared
	previewing             bool
	page                   int
	cursors                [pageCount]int
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
	clearDataOnExit        bool
	reveal                 bool
	view                   viewport.Model
	chat                   *danmakuUI
}

func New(ctx context.Context, s *store.Store) (*Model, error) {
	cfg := s.Config()
	disk, err := journal.Open(s.Dir())
	if err != nil {
		return nil, err
	}
	c, err := bili.New(cfg.Proxy)
	if err != nil {
		return nil, errors.Join(err, disk.Write(i18n.T(i18n.TUILogStartupFailed)+err.Error()), disk.Close())
	}
	if err := disk.Write(i18n.T(i18n.TUILogApplicationStarted)); err != nil {
		c.HTTP.CloseIdleConnections()
		return nil, errors.Join(err, disk.Close())
	}
	in := textinput.New()
	in.CharLimit = 4096
	m := &Model{ctx: ctx, store: s, config: cfg, client: c, journal: disk, obsClient: obs.NewClient(), width: 100, height: 32, input: in, status: i18n.T(i18n.TUIStatusReady), view: viewport.New(96, 24)}
	m.session = app.New(s, m.obsClient, c)
	m.openChat()
	return m, nil
}
func (m *Model) Init() tea.Cmd {
	if m.initialized {
		return nil
	}
	m.initialized = true
	cmds := []tea.Cmd{m.watchOBS(), chatTickCmd()}
	if m.overlay != nil && m.overlayEnabled {
		cmds = append(cmds, m.startOverlay())
	}
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
	m.status = fmt.Sprintf(i18n.T(i18n.TUIStatusOperationPending), operationName(kind))
	return func() tea.Msg { defer cancel(); value, err := fn(ctx); return resultMsg{id, kind, value, err} }
}
func operationName(kind string) string {
	switch kind {
	case "cover-prepare":
		return i18n.T(i18n.TUIOperationPrepareCover)
	case "cover-fetch":
		return i18n.T(i18n.TUIOperationFetchCover)
	case "cover-upload":
		return i18n.T(i18n.TUIOperationUploadCover)
	case "refresh":
		return i18n.T(i18n.TUIOperationRefreshRoom)
	case "account":
		return i18n.T(i18n.TUIOperationLoadAccount)
	case "qr":
		return i18n.T(i18n.TUIOperationGenerateQR)
	case "poll":
		return i18n.T(i18n.TUIOperationWaitQR)
	case "login-save":
		return i18n.T(i18n.TUIOperationSaveLogin)
	case "start":
		return i18n.T(i18n.TUIOperationStartLive)
	case "stop":
		return i18n.T(i18n.TUIOperationStopLive)
	case "areas":
		return i18n.T(i18n.TUIOperationFetchCategories)
	case "face":
		return i18n.T(i18n.TUIOperationIdentityLink)
	case "obs-connect":
		return i18n.T(i18n.TUIOBSConnect)
	case "obs-disconnect":
		return i18n.T(i18n.TUIOBSDisconnect)
	case "config", "overlay-config", "overlay-toggle":
		return i18n.T(i18n.TUIOperationSaveSettings)
	case "overlay-restore":
		return i18n.T(i18n.TUIOverlayRestore)
	case "settings-reset":
		return i18n.T(i18n.TUISettingsReset)
	case "delete":
		return i18n.T(i18n.TUIOperationRemoveAccount)
	case "delay":
		return i18n.T(i18n.TUIOperationReadDelay)
	case "area":
		return i18n.T(i18n.TUIOperationUpdateCategory)
	case "announcement":
		return i18n.T(i18n.TUIOperationUpdateAnnouncement)
	case "delay-set":
		return i18n.T(i18n.TUIOperationUpdateDelay)
	case "obs-password":
		return i18n.T(i18n.TUIOperationUpdateOBSPassword)
	case "obs-url":
		return i18n.T(i18n.TUIOperationUpdateOBSURL)
	case "title":
		return i18n.T(i18n.TUIOperationUpdateTitle)
	default:
		return fmt.Sprintf(i18n.T(i18n.TUIOperationUpdateOther), kind)
	}
}
func (m *Model) log(s string) {
	s = m.safe(s)
	if err := m.journal.Write(s); err != nil {
		s += i18n.T(i18n.TUILogWriteFailedPrefix) + m.safe(err.Error()) + i18n.T(i18n.TUILogWriteFailedSuffix)
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
				s = strings.ReplaceAll(s, v, i18n.T(i18n.TUISecurityHidden))
			}
		}
	}
	if m.stream != nil && m.stream.Key != "" {
		s = strings.ReplaceAll(s, m.stream.Key, i18n.T(i18n.TUISecurityHidden))
	}
	return clean(s)
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.clearDataOnExit {
		return m, tea.Quit
	}
	defer m.publishOverlay()
	switch msg := msg.(type) {
	case overlayStartedMsg:
		return m, m.handleOverlayStarted(msg)
	case overlayStoppedMsg:
		return m, m.handleOverlayStopped(msg)
	case overlayChatMsg:
		return m, m.handleOverlayChat(msg)
	case chatTick:
		return m, tea.Batch(m.updateChat(), m.updateOverlayChat())
	case chatPageMsg:
		return m, m.applyChatPage(msg)
	case coverPreviewFinished:
		m.previewing = false
		if msg.err != nil {
			m.log(i18n.T(i18n.TUILogCoverPreviewFailed) + msg.err.Error())
		} else {
			m.status = i18n.T(i18n.TUIStatusPreviewClosed)
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
			m.session.BeginClose()
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
				m.status = i18n.T(i18n.TUIStatusCancelRequested)
			}
			return m, nil
		}
		if m.obsBusy && key == "esc" && m.mode == "" {
			if m.obsCancel != nil {
				m.obsCancel()
			}
			_ = m.obsClient.Disconnect()
			m.status = i18n.T(i18n.TUIStatusOBSCancelRequested)
			return m, nil
		}
		if m.mode != "" {
			return m, m.modalKey(msg)
		}
		if m.page == chatPage {
			if handled, cmd := m.chatKey(key); handled {
				return m, cmd
			}
		}
		switch key {
		case "tab", "right":
			m.page = (m.page + 1) % len(m.cursors)
			m.view.GotoTop()
		case "shift+tab", "left":
			m.page = (m.page + len(m.cursors) - 1) % len(m.cursors)
			m.view.GotoTop()
		case "1", "2", "3", "4", "5", "6", "7", "8":
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
			m.pendingAccount = nil
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
			m.log(i18n.T(i18n.TUIStatusOperationCanceled))
			return nil
		}
	}
	// 即使后续本地历史保存失败，也应用已确认的远程变更。
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
	if out, ok := r.value.(app.AccountOutcome); ok && out.OldRoom != nil {
		m.room = out.OldRoom
		if !m.room.Live {
			m.stream = nil
			m.reveal = false
		}
		m.obsState = m.obsClient.Snapshot()
	}
	if r.err != nil {
		var face *domain.FaceChallenge
		if errors.As(r.err, &face) && !m.canceled {
			return m.work("face", func(ctx context.Context) (any, error) { return m.client.ResolveFace(ctx, face) })
		}
		if errors.Is(r.err, context.Canceled) {
			m.log(i18n.T(i18n.TUIStatusRemoteCancelWarning))
		} else {
			m.log(fmt.Sprintf(i18n.T(i18n.TUILogOperationFailed), operationName(r.kind), r.err.Error()))
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
	case "settings-reset":
		m.session.Release(m.client)
		m.client = r.value.(*bili.Client)
		m.overlayEnabled = m.config.Overlay.Enabled
		if m.overlay != nil {
			m.overlay.options.Config = m.config.Overlay.Config("")
		}
		m.syncChat()
		m.log(i18n.T(i18n.TUISettingsResetDone))
		return tea.Batch(m.stopOverlay(), m.updateOverlayChat())
	case "overlay-config", "overlay-toggle", "overlay-restore":
		if m.overlay == nil {
			m.overlay = &overlayRuntime{state: "off"}
		}
		m.overlay.options.Config = m.config.Overlay.Config("")
		if r.kind == "overlay-restore" {
			m.log(i18n.T(i18n.TUIOverlayRestoreDone))
		} else {
			m.log(i18n.T(i18n.TUILogSettingsSaved))
		}
		var cmd tea.Cmd
		if r.kind == "overlay-toggle" {
			m.overlayEnabled = m.config.Overlay.Enabled
			if m.overlayEnabled {
				cmd = m.startOverlay()
			} else {
				cmd = m.stopOverlay()
			}
		}
		return tea.Batch(cmd, m.updateOverlayChat())
	case "cover-prepare":
		m.cover = r.value.(*coverimage.Prepared)
		m.mode = "cover-review"
		m.view.GotoTop()
		m.status = i18n.T(i18n.TUIStatusCoverProcessed)
	case "cover-fetch":
		return m.previewCover(r.value.(image.Image), i18n.T(i18n.TUICoverCurrentPreviewTitle))
	case "cover-upload":
		update := r.value.(bili.CoverUpdate)
		m.room.CoverURL = update.URL
		m.room.CoverStatus = update.Status
		if update.Reason != "" {
			m.room.CoverStatus += " · " + m.safe(update.Reason)
		}
		m.cover = nil
		m.mode = "cover"
		m.log(i18n.T(i18n.TUILogCoverSubmitted) + m.room.CoverStatus)
	case "account", "login-save":
		a := r.value.(app.AccountOutcome)
		m.account = &a.Account
		m.session.Release(m.client)
		m.client = a.Client
		m.syncChat()
		m.room = nil
		m.stream = nil
		m.reveal = false
		m.pendingAccount = nil
		m.mode = ""
		m.qr = nil
		m.qrText = ""
		m.qrGeneration++
		m.log(i18n.T(i18n.TUILogAccountSwitched) + a.Account.Name + " / " + a.Account.UID)
		return m.refresh()
	case "refresh":
		room := r.value.(domain.Room)
		m.room = &room
		if !room.Live {
			m.stream = nil
			m.reveal = false
		}
		m.log(i18n.T(i18n.TUILogRoomRefreshed))
	case "qr":
		qr := r.value.(domain.QR)
		m.qr = &qr
		m.mode = "qr"
		m.qrGeneration++
		m.qrText = renderQR(qr.URL)
		m.view.GotoTop()
		m.status = i18n.T(i18n.TUIStatusScanQR)
		return m.nextPoll()
	case "poll":
		if m.mode != "qr" {
			return nil
		}
		poll := r.value.(domain.LoginPoll)
		switch poll.Code {
		case 0:
			if poll.Account == nil {
				m.log(i18n.T(i18n.TUILogLoginCredentialsMissing))
				m.mode = ""
				return nil
			}
			m.pendingAccount = poll.Account
			m.qr = nil
			m.qrText = ""
			m.qrGeneration++
			return m.saveLogin(*poll.Account)
		case 86101:
			m.status = i18n.T(i18n.TUIStatusWaitingQR)
		case 86090:
			m.status = i18n.T(i18n.TUIStatusConfirmLogin)
		case 86038:
			m.log(i18n.T(i18n.TUILogQRExpired))
			m.mode = ""
			m.qr = nil
			m.qrText = ""
			return nil
		default:
			m.log(fmt.Sprintf(i18n.T(i18n.TUILogLoginStatusUnexpected), poll.Code))
			m.mode = ""
			return nil
		}
		return m.nextPoll()
	case "start":
		outcome := r.value.(app.StartOutcome)
		s := outcome.Stream
		m.stream = &s
		m.reveal = false
		if m.room != nil {
			m.room.Live = true
		}
		m.log(i18n.T(i18n.TUILogLiveStarted))
		if outcome.OBSStarted {
			m.log(i18n.T(i18n.TUILogLiveOBSStarted))
		} else if outcome.OBSConfigured {
			m.log(i18n.T(i18n.TUILogLiveOBSConfigured))
		}
		if outcome.OBSErr != nil {
			m.log(i18n.T(i18n.TUILogLiveOBSFailed) + outcome.OBSErr.Error())
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
		m.log(i18n.T(i18n.TUILogLiveStopped))
		if outcome, ok := r.value.(app.StopOutcome); ok {
			if outcome.OBSStopped {
				m.log(i18n.T(i18n.TUILogLiveOBSStopped))
			}
		}
	case "face":
		m.faceURL = r.value.(string)
		m.qrText = renderQR(m.faceURL)
		m.mode = "face"
		m.view.GotoTop()
		m.status = i18n.T(i18n.TUIStatusIdentityRequired)
	case "areas":
		catalog := r.value.(areaCatalog)
		m.selection = newAreaSelection(catalog.all, catalog.recent, m.room.AreaID)
		m.mode = "selection"
		m.view.GotoTop()
		m.status = i18n.T(i18n.TUIStatusCategorySelectHint)
		if catalog.historyErr != nil {
			m.log(i18n.T(i18n.TUILogRecentCategoriesFailed) + catalog.historyErr.Error())
		}
		return m.selection.Init()
	case "delay":
		return m.form("delay", i18n.T(i18n.TUIFormDelay), fmt.Sprint(r.value.(int)), false)
	case "delete":
		out := r.value.(app.AccountOutcome)
		if out.Client != nil {
			m.account = nil
			m.room = nil
			m.stream = nil
			m.reveal = false
			m.session.Release(m.client)
			m.client = out.Client
			m.syncChat()
		}
		m.log(i18n.T(i18n.TUILogAccountRemoved))
	case "config":
		if c, ok := r.value.(*bili.Client); ok {
			m.session.Release(m.client)
			m.client = c
		}
		m.log(i18n.T(i18n.TUILogSettingsSaved))
		m.syncChat()
	default:
		m.log(fmt.Sprintf(i18n.T(i18n.TUILogOperationSucceeded), operationName(r.kind)))
	}
	m.view.SetContent(m.content())
	return m.updateOverlayChat()
}
func (m *Model) nextPoll() tea.Cmd {
	id := m.qrGeneration
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return pollTick{id} })
}
