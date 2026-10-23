package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
	"arcana-world/internal/presentation"
	"arcana-world/internal/tts"
	"arcana-world/internal/tts/models"
	tea "charm.land/bubbletea/v2"
)

type ttsRetired struct {
	done chan struct{}
	err  error
}

type ttsRuntime struct {
	enabled                   bool
	preview                   bool
	manager                   *tts.Manager
	listeningDisabled         bool
	cancel                    context.CancelFunc
	retired                   []*ttsRetired
	listener                  *danmaku.Listener
	history                   *danmaku.History
	account                   string
	room                      int64
	generation                uint64
	identity, proxy           string
	volume                    int
	disabled                  []string
	request, latest, revision uint64
	loaded, loading           bool
	lastError                 string
	failed                    bool
	modelJob                  *ttsModelJob
	modelStatus               models.Status
	modelPromptShown          map[string]bool
	modelConfirmProvider      string
	secrets                   *ttsSecretsJob
	secretSaving              bool
}

type ttsEventsMsg struct {
	owner                                 *ttsRuntime
	request, generation, latest, revision uint64
	room                                  int64
	baseline                              bool
	events                                []danmaku.Event
	err                                   error
}

// 立即取消任务，后台回收辅助进程，不阻塞界面事件循环。
func (r *ttsRuntime) retire() {
	if r.manager == nil {
		return
	}
	r.cancel()
	manager := r.manager
	retired := &ttsRetired{done: make(chan struct{})}
	r.retired = append(r.retired, retired)
	r.manager, r.cancel = nil, nil
	go func() {
		retired.err = manager.Close()
		close(retired.done)
	}()
}

// ttsError 对界面按相同文本去重；失败原因已由产生处（tts 组件与 startTTSManager）记录，
// 此处仅以 DEBUG 记录界面实际展示的错误。
func (m *Model) ttsError(err error) {
	if err == nil {
		return
	}
	detail := m.safe(err.Error())
	if m.tts.lastError == detail {
		return
	}
	m.tts.lastError = detail
	logger.Debug("tts error shown", "err", err)
	m.warn(fmt.Sprintf(i18n.T(i18n.TTSLogFailed), detail))
}

// ConfigureTTS 在模型初始化前设置仅限本次会话的覆盖值。
func (m *Model) ConfigureTTS(enabled bool) error {
	if m.initialized || m.ttsOverride != nil {
		return errors.New("TTS must be configured once before model initialization")
	}
	m.ttsOverride = &enabled
	if m.tts == nil {
		m.tts = &ttsRuntime{}
	}
	m.tts.enabled = enabled
	return nil
}

func (m *Model) syncTTS() {
	r := m.tts
	if r == nil {
		r = &ttsRuntime{}
		m.tts = r
	}
	var listener *danmaku.Listener
	var history *danmaku.History
	var account string
	var room int64
	var generation uint64
	if m.account != nil {
		account = m.account.UID
	}
	if r.enabled && !m.config.DanmakuDisabled && account != "" && m.chat != nil {
		listener, history = m.chat.listener, m.chat.history
		if listener != nil && history != nil {
			state := listener.Snapshot()
			generation = state.Generation
			if state.AccountUID == account && state.Phase != danmaku.PhaseDisabled {
				room = state.RoomID
			}
		}
	}
	identity := m.config.TTS.Identity()
	if r.listener != listener || r.history != history || r.account != account || r.room != room || r.generation != generation || r.identity != identity || r.volume != m.config.TTS.Volume || r.proxy != m.config.Proxy || r.listeningDisabled != m.config.DanmakuDisabled || !slices.Equal(r.disabled, m.config.TTS.DisabledEvents) {
		r.retire()
		if r.account != account {
			r.preview = false
		}
		if r.identity != identity {
			r.secrets = nil
			r.preview = false
			if provider, ok := tts.ProviderByID(m.config.TTS.Provider); ok && provider.Model {
				r.modelStatus = models.Inspect(m.appDir, provider.ID)
			}
		}
		if r.modelJob != nil && r.modelJob.provider != m.config.TTS.Provider {
			r.modelJob.cancel()
		}
		r.listener, r.history, r.account, r.room, r.generation = listener, history, account, room, generation
		r.identity, r.proxy = identity, m.config.Proxy
		r.volume = m.config.TTS.Volume
		r.listeningDisabled = m.config.DanmakuDisabled
		r.disabled = slices.Clone(m.config.TTS.DisabledEvents)
		r.request++
		r.latest, r.revision = 0, 0
		r.loaded, r.loading, r.failed = false, false, false
	}
	pending := r.retired[:0]
	for _, retired := range r.retired {
		select {
		case <-retired.done:
			m.ttsError(retired.err)
		default:
			pending = append(pending, retired)
		}
	}
	clear(r.retired[len(pending):])
	r.retired = pending
}

func (m *Model) startTTSManager() {
	if m.ttsOverride != nil && !*m.ttsOverride {
		return
	}
	r := m.tts
	if r.manager != nil || r.failed || r.secretSaving || len(r.retired) != 0 || r.modelJob != nil || (!r.preview && (!r.enabled || m.config.DanmakuDisabled)) {
		return
	}
	synthesizer, ready, err := m.newTTSSynthesizer()
	if !ready {
		return
	}
	if err != nil {
		r.failed = true
		logger.Error("tts synthesis backend unavailable", "err", err)
		m.ttsError(err)
		return
	}
	player, err := tts.NewPlayer(tts.PlayerOptions{Volume: m.config.TTS.Volume})
	if err != nil {
		_ = synthesizer.Close()
		r.failed = true
		logger.Error("tts playback backend unavailable", "err", err)
		m.ttsError(err)
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	manager, err := tts.New(ctx, tts.Options{Synthesizer: synthesizer, Player: player})
	if err != nil {
		cancel()
		_ = synthesizer.Close()
		_ = player.Close()
		r.failed = true
		logger.Error("tts manager start failed", "err", err)
		m.ttsError(err)
		return
	}
	r.manager, r.cancel = manager, cancel
	r.lastError = ""
	logger.Info("tts manager started")
}

func (m *Model) performTTS(action string) tea.Cmd {
	if m.ttsOverride != nil && (action == "tts-toggle" || action == "tts-preview" && !*m.ttsOverride) {
		rejectAction("tts overridden by command line for this session")
		m.warn(i18n.T(i18n.TUISessionOverride))
		return nil
	}
	if m.tts == nil {
		m.tts = &ttsRuntime{}
	}
	m.syncTTS()
	r := m.tts
	switch action {
	case "tts-model-install", "tts-model-install-confirmed", "tts-model-verify", "tts-model-cancel":
		return m.performTTSModel(action)
	case "tts-toggle":
		r.enabled = !r.enabled
		r.preview = false
		if !r.enabled {
			r.retire()
		}
	case "tts-stop":
		r.preview = false
		r.retire()
	case "tts-preview":
		// 空闲时复用已加载的模型；忙碌时先取消旧播报，避免试听排在队尾。
		if r.manager != nil {
			state := r.manager.Snapshot()
			if state.Phase != tts.PhaseIdle || state.Pending != 0 {
				r.retire()
			}
		}
		r.preview = true
	default:
		return nil
	}
	r.failed, r.loading, r.loaded = false, false, false
	r.request++
	return m.updateTTS()
}

func (m *Model) updateTTS() tea.Cmd {
	m.syncTTS()
	r := m.tts
	if r == nil {
		return nil
	}
	m.pollTTSModel()
	m.startTTSManager()
	if r.manager != nil {
		m.ttsError(r.manager.Snapshot().LastError)
		if r.preview {
			r.preview = false
			text := "你好，这是语音播报试听。欢迎来到直播间。"
			if m.config.TTS.Provider == "piper" {
				text = "Hello, this is a speech preview. Welcome to the live stream."
			}
			m.ttsError(r.manager.Enqueue(text))
		}
	}
	if !r.enabled || r.room <= 0 || r.history == nil || r.listener == nil || r.loading || r.manager == nil {
		return nil
	}
	revision := r.listener.Snapshot().Revision
	if r.loaded && r.revision == revision {
		return nil
	}
	r.loading = true
	r.request++
	msg := ttsEventsMsg{owner: r, request: r.request, generation: r.generation, room: r.room, revision: revision, baseline: !r.loaded, latest: r.latest}
	history, after := r.history, r.latest
	return func() tea.Msg {
		limit := 64
		if msg.baseline {
			limit = 1
		}
		events, err := history.Page(msg.room, 0, limit)
		msg.err = err
		if len(events) != 0 {
			msg.latest = max(after, events[0].Sequence)
		}
		if !msg.baseline {
			count := 0
			for count < len(events) && events[count].Sequence > after {
				count++
			}
			msg.events = events[:count]
			slices.Reverse(msg.events)
		}
		return msg
	}
}

func (m *Model) handleTTSEvents(msg ttsEventsMsg) tea.Cmd {
	m.syncTTS()
	r := m.tts
	if r == nil || msg.owner != r || !r.loading || msg.request != r.request || msg.generation != r.generation || msg.room != r.room {
		return nil
	}
	r.loading = false
	if msg.err != nil {
		logger.Error("tts history read failed", "room", r.room, "err", msg.err)
		m.ttsError(fmt.Errorf(i18n.T(i18n.TTSReadEventsFailed), msg.err))
		return nil
	}
	r.loaded, r.latest, r.revision = true, msg.latest, msg.revision
	if !r.enabled || r.manager == nil {
		return nil
	}
	blocked := m.config.Blocklist.TTSMatcher()
	for _, event := range msg.events {
		if event.Deleted || event.RoomID != r.room || !presentation.Enabled(r.disabled, event) || blocked.Blocks(event) {
			continue
		}
		rendered, err := m.outputTemplates.RenderTTS(event)
		if err != nil {
			m.warnOutputTemplates(err)
		}
		text := overlayChatPlain(rendered, tts.MaxTextRunes-1)
		if text == "" {
			continue
		}
		if err := r.manager.Enqueue(text); err != nil && !errors.Is(err, tts.ErrQueueFull) {
			m.ttsError(err)
		}
	}
	return nil
}

func (m *Model) ttsStateText() string {
	r := m.tts
	if r == nil {
		return fmt.Sprintf(i18n.T(i18n.TTSState), i18n.T(i18n.TTSOff))
	}
	state := i18n.T(i18n.TTSOff)
	if r.enabled {
		state = i18n.T(i18n.TTSWaitingForListener)
		if r.room > 0 {
			state = i18n.T(i18n.TTSWaitingForEvents)
		}
	}
	if len(r.retired) != 0 {
		state = i18n.T(i18n.TTSStopping)
	}
	if r.manager != nil {
		snapshot := r.manager.Snapshot()
		switch snapshot.Phase {
		case tts.PhaseSynthesizing:
			state = i18n.T(i18n.TTSSynthesizing)
		case tts.PhasePlaying:
			state = i18n.T(i18n.TTSPlaying)
		}
		state += fmt.Sprintf(i18n.T(i18n.TTSQueueCounts), snapshot.Pending, snapshot.Dropped)
	}
	if r.lastError != "" {
		state += fmt.Sprintf(i18n.T(i18n.TTSLastError), r.lastError)
	}
	return fmt.Sprintf(i18n.T(i18n.TTSState), state)
}

func (m *Model) closeTTS() error {
	r := m.tts
	if r == nil {
		return nil
	}
	if r.modelJob != nil {
		r.modelJob.cancel()
		<-r.modelJob.done
		r.modelJob = nil
	}
	r.enabled, r.preview, r.loading = false, false, false
	r.request++
	r.retire()
	var err error
	for _, retired := range r.retired {
		<-retired.done
		err = errors.Join(err, retired.err)
	}
	r.retired = nil
	return err
}
