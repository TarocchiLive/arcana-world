package tui

import (
	"context"
	"errors"
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
	"arcana-world/internal/overlay"
	"arcana-world/internal/store"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

const (
	livePage = iota
	chatPage
	overlayPage
	ttsPage
	accountsPage
	roomPage
	obsPage
	settingsPage
	logsPage
	helpPage
	pageCount
)

const (
	operationTimeout = 60 * time.Second
	qrPollInterval   = 2 * time.Second
	logEntryLimit    = 200
)

func pageNames() [pageCount]string {
	return [pageCount]string{
		livePage:     i18n.T(i18n.TUIPageLive),
		chatPage:     i18n.T(i18n.DanmakuPage),
		overlayPage:  i18n.T(i18n.OutputOverlayPage),
		ttsPage:      i18n.T(i18n.OutputTTSPage),
		accountsPage: i18n.T(i18n.TUIPageAccounts),
		roomPage:     i18n.T(i18n.TUIPageRoom),
		obsPage:      "OBS",
		settingsPage: i18n.T(i18n.TUIPageSettings),
		logsPage:     i18n.T(i18n.TUIPageLogs),
		helpPage:     i18n.T(i18n.TUIPageHelp),
	}
}

type pollTick struct{ generation int }
type choice struct{ label, value string }

type Model struct {
	ctx                    context.Context
	store                  *store.Store
	client                 *bili.Client
	config                 domain.Config
	theme                  tuiTheme
	themeID                string
	darkBackground         bool
	showcase               showcaseState
	blurred                bool
	mouseTargets           []mouseTarget
	mouseScrolling         bool
	mouseX, mouseY         int
	mouseKnown             bool
	pointerShape           string
	frame                  renderFrame
	exitPrompt             *exitConfirmation
	backdrop               string
	backdropWidth          int
	backdropHeight         int
	backdropFooterHeight   int
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
	tts                    *ttsRuntime
	ttsOverride            *bool
	overlayEnabled         bool
	overlayColorPreview    *overlay.Colors
	overlaySettings        *overlaySettingsNavigation
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
	pickerTop              int
	pickerLeft             int
	overlayDisplays        []overlay.Display
	confirmAction          string
	busy                   bool
	canceled               bool
	cancel                 context.CancelFunc
	operation              int
	qrGeneration           int
	qr                     *domain.QR
	qrText                 string
	qrImageOpening         bool
	faceURL                string
	pendingAccount         *domain.Account
	status                 string
	statusWarning          bool
	notification           notificationState
	noticeLayer            floatingLayer
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
	in.SetVirtualCursor(true)
	in.CharLimit = 4096
	m := &Model{ctx: ctx, store: s, config: cfg, client: c, journal: disk, obsClient: obs.NewClient(), width: 100, height: 32, input: in, status: i18n.T(i18n.TUIStatusReady), view: viewport.New(viewport.WithWidth(96), viewport.WithHeight(24))}
	m.darkBackground = true
	m.session = app.New(s, m.obsClient, c)
	m.openChat()
	m.logCredentialStorage()
	m.syncWorkspace()
	return m, nil
}
func (m *Model) Init() tea.Cmd {
	if m.initialized {
		return nil
	}
	m.initialized = true
	cmds := []tea.Cmd{tea.RequestBackgroundColor, m.watchOBS(), chatTickCmd(), m.syncShowcase(), m.notificationCommand(), m.pointerCommand()}
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

func (m *Model) logCredentialStorage() {
	switch m.store.CredentialStorage() {
	case store.StorageFile:
		m.warn(i18n.T(i18n.StoreFileStorageFallback))
	case store.StorageMemory:
		m.warn(i18n.T(i18n.StoreMemoryStorage))
	}
}

func (m *Model) log(s string) { m.logStatus(s, false) }

func (m *Model) logStatus(s string, warning bool) {
	s = m.safe(s)
	err := m.journal.Write(s)
	if err != nil {
		s += i18n.T(i18n.TUILogWriteFailedPrefix) + m.safe(err.Error()) + i18n.T(i18n.TUILogWriteFailedSuffix)
	}
	m.logs = append(m.logs, time.Now().Format("15:04:05")+"  "+s)
	if len(m.logs) > logEntryLimit {
		m.logs = append([]string(nil), m.logs[len(m.logs)-logEntryLimit:]...)
	}
	m.status, m.statusWarning = s, warning || err != nil
	m.resetNotification()
}

func (m *Model) setStatus(text string) {
	m.status = text
	m.statusWarning = false
	m.resetNotification()
}

func (m *Model) warnStatus(text string) {
	m.status, m.statusWarning = text, true
	m.resetNotification()
}

func (m *Model) warn(text string) { m.logStatus(text, true) }
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

// 业务轮询始终运行；仅可见状态变化时才使整页缓存失效。
func (m *Model) updatePolling() tea.Cmd {
	cmds := []tea.Cmd{m.updateChat(), m.updateOverlayChat(), m.updateTTS()}
	m.publishOverlay()
	if m.frame.base != "" && m.frame.poll == m.pollViewState() {
		m.frame.reuse = true
	} else {
		m.frame = renderFrame{}
		m.mouseTargets = m.mouseTargets[:0]
		m.noticeLayer = floatingLayer{}
		m.syncWorkspace()
		if m.initialized {
			cmds = append(cmds, refreshPointer)
		}
	}
	return tea.Batch(append(cmds, m.notificationCommand())...)
}
func (m *Model) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	// 原始终端输出已由 Bubble Tea 执行，不改变界面状态，也不再次设置指针。
	switch msg.(type) {
	case pointerRefreshMsg:
		m.frame.reuse = m.frame.base != ""
		return m, m.pointerCommand()
	case tea.RawMsg:
		m.frame.reuse = m.frame.base != ""
		return m, nil
	}
	if motion, ok := msg.(tea.MouseMotionMsg); ok {
		m.mouseX, m.mouseY, m.mouseKnown = motion.X, motion.Y, true
		m.frame.reuse = m.frame.base != ""
		return m, m.pointerCommand()
	}
	if tick, ok := msg.(showcaseTick); ok {
		frame := m.showcase.frame
		cmd := m.updateShowcase(tick)
		m.frame.reuse = m.frame.base != ""
		m.frame.separatorDirty = m.frame.separatorDirty || frame != m.showcase.frame
		return m, cmd
	}
	switch msg.(type) {
	case tea.BlurMsg:
		m.blurred = true
		m.frame.reuse = m.frame.base != ""
		return m, m.syncShowcase()
	case tea.FocusMsg:
		m.blurred = false
		m.frame.reuse = m.frame.base != ""
		return m, m.syncShowcase()
	}
	if m.deferUntilQuitResolved(msg) {
		m.frame.reuse = m.frame.base != ""
		return m, nil
	}
	if _, ok := msg.(chatTick); ok {
		return m, m.updatePolling()
	}
	// 输入和业务结果可能改变布局；只有无变化的轮询与装饰更新复用画面。
	m.frame = renderFrame{}
	if _, mouse := msg.(tea.MouseMsg); !mouse {
		m.mouseTargets = m.mouseTargets[:0]
		m.noticeLayer = floatingLayer{}
	}
	defer func() {
		m.syncWorkspace()
		if next := m.syncShowcase(); next != nil {
			cmd = tea.Batch(cmd, next)
		}
		if m.initialized {
			cmd = tea.Batch(cmd, m.notificationCommand(), refreshPointer)
		}
	}()
	if m.clearDataOnExit {
		return m, tea.Quit
	}
	defer m.publishOverlay()
	defer m.syncTTS()
	switch msg := msg.(type) {
	case notificationExpired:
		return m, m.updateNotification(msg)
	case tea.BackgroundColorMsg:
		if dark := msg.IsDark(); dark != m.darkBackground {
			m.darkBackground = dark
			m.theme = themeFor(m.config.TUITheme, dark)
			m.themeID = m.config.TUITheme
			m.backdrop = ""
		}
		return m, nil
	case tea.PasteMsg:
		if m.busy || m.previewing {
			return m, nil
		}
	case qrImageOpened:
		m.qrImageOpening = false
		if msg.err != nil {
			m.warn(i18n.T(i18n.TUIQROpenFailed) + msg.err.Error())
		}
		return m, nil
	case overlayStartedMsg:
		return m, m.handleOverlayStarted(msg)
	case overlayStoppedMsg:
		return m, m.handleOverlayStopped(msg)
	case overlayChatMsg:
		return m, m.handleOverlayChat(msg)
	case ttsEventsMsg:
		return m, m.handleTTSEvents(msg)
	case chatPageMsg:
		return m, m.applyChatPage(msg)
	case coverPreviewFinished:
		m.previewing = false
		if msg.err != nil {
			m.warn(i18n.T(i18n.TUILogCoverPreviewFailed) + msg.err.Error())
		} else {
			m.setStatus(i18n.T(i18n.TUIStatusPreviewClosed))
		}
		return m, nil
	case obsEventMsg:
		return m, m.handleOBSEvent(msg)
	case obsResultMsg:
		return m, m.handleOBSResult(msg)
	case tea.WindowSizeMsg:
		if m.page == chatPage && m.mode == "" && m.chat != nil && m.chat.follow && m.view.AtBottom() {
			m.chat.scrollToLatest = true
		}
		m.width, m.height = msg.Width, msg.Height
		m.syncWorkspace()
	case taskMessage:
		if msg.taskID() != m.operation {
			return m, nil
		}
		m.busy = false
		m.cancel = nil
		return m, msg.apply(m)
	case pollTick:
		if msg.generation == m.qrGeneration && m.mode == "qr" && !m.busy && m.qr != nil {
			key := m.qr.Key
			return m, work(m, pollOperation(), func(ctx context.Context) (domain.LoginPoll, error) { return m.client.PollQR(ctx, key) })
		}
	case tea.MouseMsg:
		return m, m.mouse(msg)
	case tea.KeyPressMsg:
		m.mouseScrolling = false
		key := msg.String()
		if key == "ctrl+c" || (key == "q" && m.mode == "") {
			return m, m.requestQuit()
		}
		if m.exitPrompt != nil {
			return m, m.quitKey(msg)
		}
		if m.previewing {
			return m, nil
		}
		if key == "o" && (m.mode == "qr" || m.mode == "face") {
			return m, m.openQRImage()
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
				m.progressStatus(i18n.T(i18n.TUIStatusCancelRequested))
				m.clearOverlayColorPreview()
				if m.overlaySettings != nil {
					return m, m.showOverlaySettings()
				}
			}
			return m, nil
		}
		if m.obsBusy && key == "esc" && m.mode == "" {
			if m.obsCancel != nil {
				m.obsCancel()
			}
			_ = m.obsClient.Disconnect()
			m.progressStatus(i18n.T(i18n.TUIStatusOBSCancelRequested))
			return m, nil
		}
		if key == "esc" && m.mode == "" && m.dismissNotification() {
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
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			m.page = int(key[0] - '1')
			m.view.GotoTop()
		case "0":
			m.page = helpPage
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
		case "space":
			if m.page == overlayPage || m.page == ttsPage {
				return m, m.activate()
			}
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
	m.input, cmd = m.input.Update(msg)
	m.updateOverlayColorPreview()
	return m, cmd
}
func (m *Model) modalKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.exitPrompt != nil {
		return m.quitKey(msg)
	}
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
	if key == "esc" && m.mode == "pick" && m.editKind == "chat-actions" {
		m.restoreChatActions()
		return nil
	}
	if key == "esc" && m.overlaySettings != nil {
		if m.mode == "pick" && m.editKind == "overlay-fields" {
			m.overlaySettings = nil
		} else {
			m.clearOverlayColorPreview()
			return m.showOverlaySettings()
		}
	}
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
		m.clearOverlayColorPreview()
		return nil
	}
	switch m.mode {
	case "form":
		if key == "enter" {
			return m.submitForm()
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.updateOverlayColorPreview()
		return cmd
	case "pick":
		switch key {
		case "r":
			if m.editKind == "overlay-displays" {
				return m.pickOverlayDisplays()
			}
		case "up", "k":
			m.selected = max(0, m.selected-1)
		case "down", "j":
			m.selected = min(len(m.choices)-1, m.selected+1)
		case "pgup":
			m.selected = max(0, m.selected-m.pickerWindow())
		case "pgdown":
			m.selected = min(len(m.choices)-1, m.selected+m.pickerWindow())
		case "home":
			m.selected = 0
		case "end":
			m.selected = len(m.choices) - 1
		case "enter":
			return m.choose()
		case "space":
			if m.editKind == "output-events" || m.editKind == "overlay-displays" {
				return m.choose()
			}
		}
	case "confirm":
		switch key {
		case "left", "right", "up", "down", "tab", "h", "l", "j", "k":
			m.selected = 1 - m.selected
		case "enter":
			action := m.confirmAction
			m.mode = ""
			if m.selected == 1 {
				return m.perform(action)
			}
			m.pendingAccount = nil
			if m.overlaySettings != nil {
				return m.showOverlaySettings()
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
func (m *Model) nextPoll() tea.Cmd {
	id := m.qrGeneration
	return tea.Tick(qrPollInterval, func(time.Time) tea.Msg { return pollTick{id} })
}
