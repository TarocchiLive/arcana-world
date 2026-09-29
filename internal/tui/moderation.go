package tui

import (
	"context"
	"fmt"
	"strconv"

	"arcana-world/internal/bili"
	"arcana-world/internal/i18n"
	"arcana-world/internal/termimage"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type moderationKind uint8

const (
	moderationAdmins moderationKind = iota + 1
	moderationBlocks
	moderationMutes
)

type moderationAction uint8

const (
	moderationAddAdmin moderationAction = iota + 1
	moderationRemoveAdmin
	moderationAddBlock
	moderationRemoveBlock
	moderationAddMute
	moderationRemoveMute
)

type moderationState struct {
	client     *bili.Client
	account    string
	roomID     int64
	kind       moderationKind
	action     moderationAction
	users      []bili.RoomUser
	user       bili.RoomUser
	query      string
	avatar     *termimage.Thumbnail
	pending    bool
	hours      int64
	draft      string
	parentView viewport.Model
	selected   int
}

func (m *Model) moderationValid(s *moderationState) bool {
	return s != nil && m.moderation == s && m.client == s.client && m.account != nil &&
		m.account.UID == s.account && m.config.ActiveUID == s.account && m.room != nil && m.room.ID == s.roomID
}

func (m *Model) openModeration(kind moderationKind) tea.Cmd {
	if m.busy || m.client == nil || (kind != moderationAdmins && kind != moderationBlocks && kind != moderationMutes) || !m.requireRoom() {
		return nil
	}
	m.moderation = &moderationState{client: m.client, account: m.config.ActiveUID, roomID: m.room.ID, kind: kind, parentView: m.view}
	return m.moderationMenu()
}

func (m *Model) moderationMenu() tea.Cmd {
	s := m.moderation
	if !m.moderationValid(s) {
		m.mode = ""
		return nil
	}
	s.pending = false
	label, add, list := i18n.ModerationAdmins, i18n.ModerationAddAdmin, i18n.ModerationList
	if s.kind == moderationBlocks {
		label, add, list = i18n.ModerationBlocks, i18n.ModerationAddBlock, i18n.ModerationBlockList
	}
	if s.kind == moderationMutes {
		label, add, list = i18n.ModerationMutes, i18n.ModerationAddMute, i18n.ModerationMuteList
	}
	m.choices = []choice{{i18n.T(list), "list"}, {i18n.T(add), "search"}}
	return m.pick("moderation-menu", i18n.T(label))
}

func (m *Model) chooseModeration(value string) tea.Cmd {
	s := m.moderation
	if !m.moderationValid(s) {
		m.mode = ""
		return nil
	}
	if m.editKind == "moderation-menu" {
		switch value {
		case "search":
			return m.form("moderation-search", i18n.T(i18n.ModerationSearch), s.query, false)
		case "list":
			return m.loadModerationUsers("", false)
		}
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < 0 || index >= len(s.users) {
		return nil
	}
	s.selected = index
	s.user = s.users[index]
	return m.loadModerationAvatar()
}

func (m *Model) moderationUsers() tea.Cmd {
	s := m.moderation
	if !m.moderationValid(s) {
		return nil
	}
	m.choices = make([]choice, 0, len(s.users))
	for i, user := range s.users {
		m.choices = append(m.choices, choice{fmt.Sprintf("%s · UID %d", selectionText(user.Name), user.UID), strconv.Itoa(i)})
	}
	title := i18n.ModerationAdminListTitle
	if s.action == moderationRemoveBlock {
		title = i18n.ModerationBlockListTitle
	}
	if s.action == moderationRemoveMute {
		title = i18n.ModerationMuteListTitle
	}
	cmd := m.pick("moderation-users", i18n.T(title))
	m.selected = min(s.selected, max(0, len(s.users)-1))
	return cmd
}

func (m *Model) loadModerationUsers(name string, refresh bool) tea.Cmd {
	s := m.moderation
	if m.busy || !m.moderationValid(s) {
		return nil
	}
	switch s.kind {
	case moderationAdmins:
		s.action = moderationRemoveAdmin
		if name != "" {
			s.action = moderationAddAdmin
		}
	case moderationBlocks:
		s.action = moderationRemoveBlock
		if name != "" {
			s.action = moderationAddBlock
		}
	case moderationMutes:
		s.action = moderationRemoveMute
		if name != "" {
			s.action = moderationAddMute
		}
	default:
		return nil
	}
	s.pending = false
	s.query = name
	m.input.Blur()
	op := operation[[]bili.RoomUser]{label: i18n.ModerationSearch, discardOnCancel: true, handle: func(m *Model, users []bili.RoomUser, err error, label i18n.Key) tea.Cmd {
		if !m.moderationValid(s) {
			return nil
		}
		if err != nil {
			m.warn(fmt.Sprintf(i18n.T(i18n.TUILogOperationFailed), i18n.T(label), err))
			return m.moderationMenu()
		}
		s.users = users
		if len(users) == 0 {
			if !refresh {
				m.warnStatus(i18n.T(i18n.ModerationEmpty))
			}
			if name != "" {
				return m.form("moderation-search", i18n.T(i18n.ModerationSearch), name, false)
			}
			return m.moderationMenu()
		}
		if name != "" {
			var matches []bili.RoomUser
			for _, user := range users {
				if user.Name == name || strconv.FormatInt(user.UID, 10) == name {
					matches = append(matches, user)
				}
			}
			if len(matches) == 1 {
				s.user = matches[0]
			} else if len(users) == 1 {
				s.user = users[0]
			} else {
				m.warnStatus(i18n.T(i18n.ModerationAmbiguous))
				return m.form("moderation-search", i18n.T(i18n.ModerationSearch), name, false)
			}
			return m.loadModerationAvatar()
		}
		return m.moderationUsers()
	}}
	return work(m, op, func(ctx context.Context) ([]bili.RoomUser, error) {
		if name != "" {
			return s.client.SearchRoomUsers(ctx, name)
		}
		if s.kind == moderationAdmins {
			return s.client.RoomAdmins(ctx, s.roomID)
		}
		if s.kind == moderationMutes {
			return s.client.RoomMutes(ctx, s.roomID)
		}
		return s.client.RoomBlocks(ctx, s.roomID)
	})
}

func moderationActionLabel(action moderationAction) i18n.Key {
	switch action {
	case moderationAddAdmin:
		return i18n.ModerationAddAdmin
	case moderationRemoveAdmin:
		return i18n.ModerationRemoveAdmin
	case moderationAddBlock:
		return i18n.ModerationAddBlock
	case moderationRemoveBlock:
		return i18n.ModerationRemoveBlock
	case moderationAddMute:
		return i18n.ModerationAddMute
	case moderationRemoveMute:
		return i18n.ModerationRemoveMute
	default:
		return ""
	}
}

func (m *Model) confirmModeration() tea.Cmd {
	s := m.moderation
	if !m.moderationValid(s) || s.user.UID <= 0 || moderationActionLabel(s.action) == "" {
		return nil
	}
	prompt := fmt.Sprintf(i18n.T(i18n.ModerationConfirm), i18n.T(moderationActionLabel(s.action)), selectionText(s.user.Name), s.user.UID, s.roomID)
	if s.action == moderationAddBlock {
		prompt += "\n" + i18n.T(i18n.ModerationBlockWarning)
	}
	if s.action == moderationAddMute {
		if s.hours != -1 && s.hours <= 0 {
			return m.pickMuteDuration()
		}
		prompt += "\n" + muteDurationLabel(s.hours)
	}
	if s.avatar == nil {
		prompt += "\n" + i18n.T(i18n.ModerationAvatarMissing)
	}
	s.pending = true
	return m.confirm(prompt, "moderation-apply")
}

func (m *Model) moderationBack() tea.Cmd {
	s := m.moderation
	if !m.moderationValid(s) {
		m.mode = ""
		return nil
	}
	s.pending = false
	if s.action == moderationAddMute {
		return m.pickMuteDuration()
	}
	if s.action == moderationAddAdmin || s.action == moderationAddBlock {
		return m.form("moderation-search", i18n.T(i18n.ModerationSearch), s.query, false)
	}
	return m.moderationUsers()
}

func (m *Model) loadModerationAvatar() tea.Cmd {
	s := m.moderation
	if m.busy || !m.moderationValid(s) {
		return nil
	}
	s.avatar = nil
	s.pending = false
	user := s.user
	op := operation[*termimage.Thumbnail]{label: i18n.ModerationFetchAvatar, discardOnCancel: true, handle: func(m *Model, avatar *termimage.Thumbnail, err error, _ i18n.Key) tea.Cmd {
		if !m.moderationValid(s) {
			return nil
		}
		if err != nil || avatar == nil {
			m.warn(i18n.T(i18n.ModerationAvatarMissing))
		}
		s.avatar = avatar
		if s.action == moderationAddMute {
			return m.pickMuteDuration()
		}
		return m.confirmModeration()
	}}
	return work(m, op, func(ctx context.Context) (*termimage.Thumbnail, error) {
		if user.Face == "" {
			users, err := s.client.SearchRoomUsers(ctx, strconv.FormatInt(user.UID, 10))
			if err != nil {
				return nil, err
			}
			for _, match := range users {
				if match.UID == user.UID {
					user.Face = match.Face
					break
				}
			}
		}
		img, err := s.client.FetchCover(ctx, user.Face)
		if err != nil {
			return nil, err
		}
		return termimage.NewThumbnail(img)
	})
}

func (m *Model) applyModeration() tea.Cmd {
	s := m.moderation
	if m.busy || !m.moderationValid(s) || !s.pending || m.confirmAction != "moderation-apply" || m.selected != 1 || s.user.UID <= 0 || moderationActionLabel(s.action) == "" {
		return nil
	}
	s.pending = false
	user, action, hours := s.user, s.action, s.hours
	op := operation[struct{}]{label: moderationActionLabel(action), handle: func(m *Model, _ struct{}, err error, label i18n.Key) tea.Cmd {
		result := i18n.T(i18n.ModerationSuccess)
		if err != nil {
			result = err.Error()
		}
		m.logStatus(fmt.Sprintf(i18n.T(i18n.ModerationLog), s.roomID, user.UID, i18n.T(label), result), err != nil)
		if m.canceled || !m.moderationValid(s) {
			return nil
		}
		return m.loadModerationUsers("", true)
	}}
	return work(m, op, func(ctx context.Context) (struct{}, error) {
		var err error
		switch action {
		case moderationAddAdmin:
			err = s.client.AddRoomAdmin(ctx, s.roomID, user.UID)
		case moderationRemoveAdmin:
			err = s.client.RemoveRoomAdmin(ctx, s.roomID, user.UID)
		case moderationAddBlock:
			err = s.client.AddRoomBlock(ctx, s.roomID, user.UID)
		case moderationRemoveBlock:
			err = s.client.RemoveRoomBlock(ctx, s.roomID, user.UID)
		case moderationAddMute:
			err = s.client.MuteRoomUser(ctx, s.roomID, user.UID, hours)
		case moderationRemoveMute:
			err = s.client.RemoveRoomMute(ctx, s.roomID, user.UID)
		}
		return struct{}{}, err
	})
}

// 两个入口复用现有选择器和输入框，时长草稿保留在各自的操作状态中。
func (m *Model) muteDraft() *string {
	if m.speaker != nil {
		if m.speakerValid(m.speaker) && m.speaker.action == 0 {
			return &m.speaker.draft
		}
		return nil
	}
	if m.moderationValid(m.moderation) && m.moderation.action == moderationAddMute {
		return &m.moderation.draft
	}
	return nil
}

func (m *Model) pickMuteDuration() tea.Cmd {
	if m.muteDraft() == nil {
		if m.speaker != nil {
			return m.closeChatSpeaker()
		}
		m.mode = ""
		return nil
	}
	if m.speaker != nil {
		m.speaker.pending = false
	} else {
		m.moderation.pending = false
	}
	m.input.Blur()
	m.confirmAction = ""
	m.choices = []choice{{i18n.T(i18n.MuteCustomHours), "custom"}, {i18n.T(i18n.MutePermanent), "permanent"}}
	return m.pick("mute-duration", i18n.T(i18n.MuteDuration))
}

func (m *Model) chooseMuteDuration(value string) tea.Cmd {
	draft := m.muteDraft()
	if draft == nil || m.busy {
		return nil
	}
	switch value {
	case "custom":
		return m.form("mute-hours", i18n.T(i18n.MuteHoursPrompt), *draft, false)
	case "permanent":
		return m.confirmMuteDuration(-1)
	}
	return nil
}

func (m *Model) submitMuteHours() tea.Cmd {
	draft := m.muteDraft()
	if draft == nil || m.busy {
		return nil
	}
	*draft = m.input.Value()
	hours, err := strconv.ParseInt(*draft, 10, 64)
	valid := err == nil && hours > 0
	for _, r := range *draft {
		if r < '0' || r > '9' {
			valid = false
		}
	}
	if !valid {
		m.warnStatus(i18n.T(i18n.MuteHoursInvalid))
		return nil
	}
	m.input.Blur()
	return m.confirmMuteDuration(hours)
}

func (m *Model) confirmMuteDuration(hours int64) tea.Cmd {
	if m.muteDraft() == nil || (hours != -1 && hours <= 0) {
		return nil
	}
	if m.speaker != nil {
		m.speaker.hours = hours
		return m.confirmChatSpeaker()
	}
	m.moderation.hours = hours
	return m.confirmModeration()
}

func muteDurationLabel(hours int64) string {
	if hours == -1 {
		return i18n.T(i18n.MutePermanent)
	}
	return fmt.Sprintf(i18n.T(i18n.MuteHoursLabel), hours)
}

func (m *Model) muteDurationBack() tea.Cmd {
	if m.editKind == "mute-hours" {
		if draft := m.muteDraft(); draft != nil {
			*draft = m.input.Value()
		}
		return m.pickMuteDuration()
	}
	if m.speaker != nil {
		return m.returnChatSpeaker()
	}
	if m.moderationValid(m.moderation) {
		return m.form("moderation-search", i18n.T(i18n.ModerationSearch), m.moderation.query, false)
	}
	m.mode = ""
	return nil
}
