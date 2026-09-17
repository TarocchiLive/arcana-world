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
	tea "github.com/charmbracelet/bubbletea"
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
	voice, proxy              string
	disabled                  []string
	request, latest, revision uint64
	loaded, loading           bool
	lastError                 string
	failed                    bool
}

type ttsEventsMsg struct {
	owner                                 *ttsRuntime
	request, generation, latest, revision uint64
	room                                  int64
	baseline                              bool
	events                                []danmaku.Event
	err                                   error
}

// Cancellation is immediate; reaping a helper never blocks the UI event loop.
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

func (m *Model) ttsError(err error) {
	if err == nil {
		return
	}
	detail := m.safe(err.Error())
	if m.tts.lastError != detail {
		m.tts.lastError = detail
		m.log(fmt.Sprintf(i18n.T(i18n.TTSLogFailed), detail))
	}
}

func (m *Model) syncTTS() {
	r := m.tts
	if r == nil {
		return
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
	if r.listener != listener || r.history != history || r.account != account || r.room != room || r.generation != generation || r.voice != m.config.TTS.Voice || r.proxy != m.config.Proxy || r.listeningDisabled != m.config.DanmakuDisabled || !slices.Equal(r.disabled, m.config.TTS.DisabledEvents) {
		r.retire()
		r.listener, r.history, r.account, r.room, r.generation = listener, history, account, room, generation
		r.voice, r.proxy = m.config.TTS.Voice, m.config.Proxy
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
	r := m.tts
	if r.manager != nil || r.failed || len(r.retired) != 0 || (!r.preview && (!r.enabled || m.config.DanmakuDisabled)) {
		return
	}
	edge, err := tts.NewEdge(tts.EdgeOptions{Voice: r.voice, Proxy: r.proxy})
	if err != nil {
		r.failed = true
		m.ttsError(fmt.Errorf(i18n.T(i18n.TTSSynthesisBackendUnavailable), err))
		return
	}
	player, err := tts.NewPlayer(tts.PlayerOptions{})
	if err != nil {
		r.failed = true
		m.ttsError(fmt.Errorf(i18n.T(i18n.TTSPlaybackBackendUnavailable), err))
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	manager, err := tts.New(ctx, tts.Options{Synthesizer: edge, Player: player})
	if err != nil {
		cancel()
		r.failed = true
		m.ttsError(err)
		return
	}
	r.manager, r.cancel = manager, cancel
	r.lastError = ""
}

func (m *Model) performTTS(action string) tea.Cmd {
	if m.tts == nil {
		m.tts = &ttsRuntime{}
	}
	r := m.tts
	switch action {
	case "tts-toggle":
		r.enabled = !r.enabled
		r.preview = false
		r.retire()
	case "tts-stop":
		r.preview = false
		r.retire()
	case "tts-preview":
		r.retire()
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
	m.startTTSManager()
	if r.manager != nil {
		m.ttsError(r.manager.Snapshot().LastError)
		if r.preview {
			r.preview = false
			m.ttsError(r.manager.Enqueue("你好，这是语音播报试听。欢迎来到直播间。"))
		}
		state := r.manager.Snapshot()
		if !r.enabled && state.Phase == tts.PhaseIdle && state.Pending == 0 {
			r.retire()
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
		m.ttsError(fmt.Errorf(i18n.T(i18n.TTSReadEventsFailed), msg.err))
		return nil
	}
	r.loaded, r.latest, r.revision = true, msg.latest, msg.revision
	if !r.enabled || r.manager == nil {
		return nil
	}
	for _, event := range msg.events {
		if event.Deleted || event.RoomID != r.room || !presentation.Enabled(r.disabled, event) {
			continue
		}
		text := overlayChatPlain(presentation.RenderTTS(event), tts.MaxTextRunes-1)
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
