package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"arcana-world/internal/bili"
	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
	"arcana-world/internal/termimage"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type chatSpeaker struct {
	client      *bili.Client
	account     string
	roomID, uid int64
	event       danmaku.Event
	parentMode  string
	parentView  viewport.Model
	avatar      *termimage.Thumbnail
	cancel      context.CancelFunc
	selected    int
	action      int
	pending     bool
	result      string
}

type speakerAvatarMsg struct {
	speaker *chatSpeaker
	avatar  *termimage.Thumbnail
}

type speakerHomepageMsg struct {
	speaker *chatSpeaker
	err     error
}

func (m *Model) speakerValid(s *chatSpeaker) bool {
	return s != nil && m.speaker == s && m.client == s.client && m.account != nil && m.account.UID == s.account && m.config.ActiveUID == s.account && m.room != nil && m.room.ID == s.roomID
}

func (m *Model) floatingModal() bool {
	return m.mode == "confirm" || m.mode == "speaker"
}

func (m *Model) openChatSpeaker(event danmaku.Event) tea.Cmd {
	uid, err := strconv.ParseInt(event.UID, 10, 64)
	if err != nil || uid <= 0 || event.Mystery || m.busy || m.client == nil || m.account == nil || m.room == nil || event.RoomID != m.room.ID || (m.mode != "" && m.mode != "chat-history") {
		return nil
	}
	// 只接受十进制 UID，主页地址不拼接任意事件字符串。
	for _, r := range event.UID {
		if r < '0' || r > '9' {
			return nil
		}
	}
	s := &chatSpeaker{client: m.client, account: m.config.ActiveUID, roomID: m.room.ID, uid: uid, event: event, parentMode: m.mode, parentView: m.view}
	m.speaker = s
	m.mode = "speaker"
	m.view.GotoTop()
	m.frame = renderFrame{}
	m.mouseTargets = nil
	m.clearTextSelection()
	ctx, cancel := context.WithTimeout(m.ctx, operationTimeout)
	s.cancel = cancel
	return func() tea.Msg {
		defer cancel()
		face := event.Face
		if face == "" {
			users, err := s.client.SearchRoomUsers(ctx, strconv.FormatInt(uid, 10))
			if err == nil {
				for _, user := range users {
					if user.UID == uid {
						face = user.Face
						break
					}
				}
			}
		}
		var avatar *termimage.Thumbnail
		if face != "" {
			if img, err := s.client.FetchCover(ctx, face); err == nil {
				avatar, _ = termimage.NewThumbnail(img)
			}
		}
		return speakerAvatarMsg{s, avatar}
	}
}

func (m *Model) closeChatSpeaker() tea.Cmd {
	if s := m.speaker; s != nil {
		if s.cancel != nil {
			s.cancel()
		}
		m.mode, m.view = s.parentMode, s.parentView
		if m.chat != nil && s.parentMode == "" {
			m.chat.scrollToLatest = s.parentView.AtBottom()
			m.chat.shown = true
		}
		m.speaker = nil
	}
	return nil
}

func (m *Model) returnChatSpeaker() tea.Cmd {
	if !m.speakerValid(m.speaker) {
		return m.closeChatSpeaker()
	}
	m.mode = "speaker"
	m.confirmAction = ""
	m.speaker.pending = false
	m.view.GotoTop()
	return nil
}

func speakerActionLabel(action int) i18n.Key {
	return [...]i18n.Key{i18n.SpeakerMute, i18n.SpeakerBlock, i18n.SpeakerAdmin, i18n.SpeakerHomepage}[action]
}

func (m *Model) speakerIdentityView() string {
	s := m.speaker
	if s == nil {
		return ""
	}
	width := max(1, m.view.Width())
	medal := i18n.T(i18n.SpeakerMedalUnknown)
	if s.event.MedalName != "" && s.event.MedalLevel > 0 {
		medal = fmt.Sprintf("%s · Lv.%d", selectionText(s.event.MedalName), s.event.MedalLevel)
	}
	text := fmt.Sprintf("%s\nUID %d\n%s", selectionText(s.event.User), s.uid, medal)
	if s.avatar == nil {
		text += "\n" + i18n.T(i18n.ModerationAvatarMissing)
	}
	body := m.theme.textStyle.Render(ansi.Wrap(text, width, ""))
	if avatar := m.confirmationAvatarView(); avatar != "" {
		cols := m.avatarColumns()
		if width >= cols+22 {
			body = lipgloss.JoinHorizontal(lipgloss.Top, avatar, "  ", m.theme.textStyle.Render(ansi.Wrap(text, width-cols-2, "")))
		} else {
			body = lipgloss.JoinVertical(lipgloss.Left, avatar, body)
		}
	}
	return lipgloss.NewStyle().Width(width).PaddingBottom(1).Render(body)
}

func (m *Model) speakerView() string {
	if m.speaker == nil {
		return ""
	}
	rows := []string{m.speakerIdentityView()}
	for i := range 4 {
		rows = append(rows, m.theme.listRow(i18n.T(speakerActionLabel(i)), m.view.Width(), m.speaker.selected == i))
	}
	if m.speaker.result != "" {
		rows = append(rows, m.theme.hintText(m.speaker.result, m.view.Width()))
	}
	return strings.Join(rows, "\n")
}

func (m *Model) speakerKey(msg tea.KeyPressMsg) tea.Cmd {
	if !m.speakerValid(m.speaker) {
		return m.closeChatSpeaker()
	}
	switch msg.String() {
	case "esc":
		return m.closeChatSpeaker()
	case "up", "k", "shift+tab":
		m.speaker.selected = (m.speaker.selected + 3) % 4
	case "down", "j", "tab":
		m.speaker.selected = (m.speaker.selected + 1) % 4
	case "enter":
		return m.chooseChatSpeaker()
	}
	return nil
}

func (m *Model) chooseChatSpeaker() tea.Cmd {
	s := m.speaker
	if m.busy || !m.speakerValid(s) || s.selected < 0 || s.selected > 3 {
		return nil
	}
	s.action = s.selected
	if s.action == 3 {
		ctx := m.ctx
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(ctx, operationTimeout)
			defer cancel()
			url := "https://space.bilibili.com/" + strconv.FormatInt(s.uid, 10)
			var command *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				command = exec.CommandContext(ctx, "open", url)
			case "windows":
				command = exec.CommandContext(ctx, "rundll32.exe", "url.dll,FileProtocolHandler", url)
			case "linux", "freebsd", "openbsd", "netbsd":
				command = exec.CommandContext(ctx, "xdg-open", url)
			default:
				return speakerHomepageMsg{s, fmt.Errorf("opening URLs is unsupported on %s", runtime.GOOS)}
			}
			return speakerHomepageMsg{s, command.Run()}
		}
	}
	if !m.requireRoom() {
		return nil
	}
	key := [...]i18n.Key{i18n.SpeakerConfirmMute, i18n.SpeakerConfirmBlock, i18n.SpeakerConfirmAdmin}[s.action]
	s.pending = true
	return m.confirm(fmt.Sprintf(i18n.T(key), selectionText(s.event.User), s.uid), "speaker-apply")
}

func (m *Model) applyChatSpeaker() tea.Cmd {
	s := m.speaker
	if !m.speakerValid(s) || !s.pending || m.confirmAction != "speaker-apply" || m.selected != 1 || s.action < 0 || s.action > 2 || m.busy {
		return m.closeChatSpeaker()
	}
	action := s.action
	s.pending = false
	m.mode = "speaker"
	op := operation[struct{}]{label: speakerActionLabel(action), handle: func(m *Model, _ struct{}, err error, label i18n.Key) tea.Cmd {
		result := i18n.T(i18n.ModerationSuccess)
		if err != nil {
			result = err.Error()
		}
		m.logStatus(fmt.Sprintf(i18n.T(i18n.ModerationLog), s.roomID, s.uid, i18n.T(label), result), err != nil)
		if !m.speakerValid(s) {
			return nil
		}
		s.result = m.safe(result)
		return m.returnChatSpeaker()
	}}
	return work(m, op, func(ctx context.Context) (struct{}, error) {
		var err error
		switch action {
		case 0:
			err = s.client.MuteRoomUser(ctx, s.roomID, s.uid)
		case 1:
			err = s.client.AddRoomBlock(ctx, s.roomID, s.uid)
		case 2:
			err = s.client.AddRoomAdmin(ctx, s.roomID, s.uid)
		}
		return struct{}{}, err
	})
}
