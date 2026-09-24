package tui

import (
	"context"
	"fmt"
	"strconv"

	"arcana-world/internal/bili"
	"arcana-world/internal/i18n"
	"arcana-world/internal/termimage"
	tea "charm.land/bubbletea/v2"
)

type moderationKind uint8

const (
	moderationAdmins moderationKind = iota + 1
	moderationBlocks
)

type moderationAction uint8

const (
	moderationAddAdmin moderationAction = iota + 1
	moderationRemoveAdmin
	moderationAddBlock
	moderationRemoveBlock
)

type moderationState struct {
	client  *bili.Client
	account string
	roomID  int64
	kind    moderationKind
	action  moderationAction
	users   []bili.RoomUser
	user    bili.RoomUser
	query   string
	avatar  *termimage.Thumbnail
	pending bool
}

func (m *Model) moderationValid(s *moderationState) bool {
	return s != nil && m.moderation == s && m.client == s.client && m.account != nil &&
		m.account.UID == s.account && m.config.ActiveUID == s.account && m.room != nil && m.room.ID == s.roomID
}

func (m *Model) openModeration(kind moderationKind) tea.Cmd {
	if m.busy || m.client == nil || (kind != moderationAdmins && kind != moderationBlocks) || !m.requireRoom() {
		return nil
	}
	m.moderation = &moderationState{client: m.client, account: m.config.ActiveUID, roomID: m.room.ID, kind: kind}
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
			return m.form("moderation-search", i18n.T(i18n.ModerationSearch), "", false)
		case "list":
			return m.loadModerationUsers("")
		}
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < 0 || index >= len(s.users) {
		return nil
	}
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
	return m.pick("moderation-users", i18n.T(title))
}

func (m *Model) loadModerationUsers(name string) tea.Cmd {
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
			m.warnStatus(i18n.T(i18n.ModerationEmpty))
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
	user, action := s.user, s.action
	op := operation[struct{}]{label: moderationActionLabel(action), handle: func(m *Model, _ struct{}, err error, label i18n.Key) tea.Cmd {
		result := i18n.T(i18n.ModerationSuccess)
		if err != nil {
			result = err.Error()
		}
		m.logStatus(fmt.Sprintf(i18n.T(i18n.ModerationLog), s.roomID, user.UID, i18n.T(label), result), err != nil)
		if m.canceled || !m.moderationValid(s) {
			return nil
		}
		return m.loadModerationUsers("")
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
		}
		return struct{}{}, err
	})
}
