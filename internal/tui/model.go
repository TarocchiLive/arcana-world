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
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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
	m.logCredentialStorage()
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

func (m *Model) logCredentialStorage() {
	switch m.store.CredentialStorage() {
	case store.StorageFile:
		m.log(i18n.T(i18n.StoreFileStorageFallback))
	case store.StorageMemory:
		m.log(i18n.T(i18n.StoreMemoryStorage))
	}
}

func (m *Model) log(s string) {
	s = m.safe(s)
	if err := m.journal.Write(s); err != nil {
		s += i18n.T(i18n.TUILogWriteFailedPrefix) + m.safe(err.Error()) + i18n.T(i18n.TUILogWriteFailedSuffix)
	}
	m.logs = append(m.logs, time.Now().Format("15:04:05")+"  "+s)
	if len(m.logs) > logEntryLimit {
		m.logs = append([]string(nil), m.logs[len(m.logs)-logEntryLimit:]...)
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
	defer m.syncTTS()
	switch msg := msg.(type) {
	case qrImageOpened:
		m.qrImageOpening = false
		if msg.err != nil {
			m.log(i18n.T(i18n.TUIQROpenFailed) + msg.err.Error())
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
	case chatTick:
		return m, tea.Batch(m.updateChat(), m.updateOverlayChat(), m.updateTTS())
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
		return m, m.overlayDisplayMouse(msg)
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
				m.status = i18n.T(i18n.TUIStatusCancelRequested)
				m.clearOverlayColorPreview()
				if m.overlaySettings != nil {
					return m, m.showOverlaySettings()
				}
				if m.editKind == "overlay-displays" {
					return m, tea.DisableMouse
				}
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
		case " ":
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
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.updateOverlayColorPreview()
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
		if m.editKind == "overlay-displays" {
			return tea.DisableMouse
		}
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
		case " ":
			if m.editKind == "output-events" || m.editKind == "overlay-displays" {
				return m.choose()
			}
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
