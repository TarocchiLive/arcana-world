package tui

import (
	"context"
	"fmt"
	"strings"

	"arcana-world/internal/bili"
	"arcana-world/internal/i18n"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type roomMembersState struct {
	client       *bili.Client
	accountUID   string
	roomID       int64
	fleet        bool
	returnView   viewport.Model
	returnBottom bool
	list         bili.RoomMemberList
	loaded       bool
	loading      bool
	err          error
}

func (m *Model) roomMembersValid(s *roomMembersState) bool {
	return s != nil && m.members == s && m.client == s.client && m.account != nil &&
		m.account.UID == s.accountUID && m.config.ActiveUID == s.accountUID && m.room != nil && m.room.ID == s.roomID
}

func (m *Model) openRoomMembers(fleet bool) tea.Cmd {
	if m.busy || m.members != nil || m.chat == nil || !m.requireRoom() || m.client == nil {
		return nil
	}
	m.members = &roomMembersState{
		client: m.client, accountUID: m.account.UID, roomID: m.room.ID,
		fleet:      fleet,
		returnView: m.view, returnBottom: m.view.AtBottom(),
	}
	m.chatInput.Blur()
	m.mode = "members"
	m.view = viewport.New(viewport.WithWidth(m.view.Width()), viewport.WithHeight(m.view.Height()))
	return m.refreshRoomMembers()
}

func (m *Model) closeRoomMembers() {
	s := m.members
	if s == nil {
		return
	}
	m.members = nil
	m.mode = ""
	m.view = s.returnView
	if m.chat != nil {
		m.chat.scrollToLatest = s.returnBottom
		m.chat.shown = true
	}
}

func (m *Model) refreshRoomMembers() tea.Cmd {
	s := m.members
	if m.busy || !m.roomMembersValid(s) {
		return nil
	}
	s.loading, s.err = true, nil
	fetch, failed := i18n.MembersFetch, i18n.MembersLoadFailed
	if s.fleet {
		fetch, failed = i18n.FleetFetch, i18n.FleetLoadFailed
	}
	op := operation[bili.RoomMemberList]{label: fetch, discardOnCancel: true, handle: func(m *Model, list bili.RoomMemberList, err error, _ i18n.Key) tea.Cmd {
		if !m.roomMembersValid(s) {
			return nil
		}
		s.loading, s.err = false, err
		m.mode = "members"
		if err == nil {
			s.list, s.loaded = list, true
		} else {
			m.warnStatus(fmt.Sprintf(i18n.T(failed), m.safe(err.Error())))
		}
		return nil
	}}
	return work(m, op, func(ctx context.Context) (bili.RoomMemberList, error) {
		if s.fleet {
			return s.client.RoomFleet(ctx, s.roomID)
		}
		return s.client.RoomMembers(ctx, s.roomID)
	})
}

func (m *Model) roomMembersView() string {
	s := m.members
	if !m.roomMembersValid(s) {
		return ""
	}
	width := max(1, m.view.Width())
	title, loading := i18n.MembersTitle, i18n.MembersLoading
	failed, total, empty := i18n.MembersLoadFailed, i18n.MembersTotal, i18n.MembersEmpty
	if s.fleet {
		title, loading = i18n.FleetTitle, i18n.FleetLoading
		failed, total, empty = i18n.FleetLoadFailed, i18n.FleetTotal, i18n.FleetEmpty
	}
	rows := []string{
		m.theme.sectionTitle(i18n.T(title), width),
	}
	if s.loading {
		rows = append(rows, m.theme.hintText(i18n.T(loading), width))
	}
	if s.err != nil {
		rows = append(rows, m.theme.warning.Render(ansi.Wrap(fmt.Sprintf(i18n.T(failed), m.safe(s.err.Error())), width, "")))
		if s.loaded {
			rows = append(rows, m.theme.hintText(i18n.T(i18n.MembersPrevious), width))
		}
	}
	// 首次加载失败时不渲染空名单，刷新失败时保留上次成功结果。
	if !s.loaded {
		return strings.Join(rows, "\n")
	}
	count := s.list.Total
	ignoreSelf := !s.fleet && !m.config.AudienceIncludeSelf
	if !s.fleet {
		count = m.visibleAudienceCount(count)
	}
	rows = append(rows, m.theme.hintText(fmt.Sprintf(i18n.T(total), count), width), "")
	visible := 0
	for _, member := range s.list.Members {
		if ignoreSelf && member.Self {
			continue
		}
		visible++
		var role i18n.Key
		switch member.GuardLevel {
		case 1:
			role = i18n.MembersGovernor
		case 2:
			role = i18n.MembersAdmiral
		case 3:
			role = i18n.MembersCaptain
		}
		name := fmt.Sprintf("#%d  %s", member.Rank, selectionText(member.Name))
		if role != "" {
			name += " · " + i18n.T(role)
		}
		detail := ""
		if !member.Mystery && member.UID > 0 {
			detail = fmt.Sprintf("UID %d", member.UID)
		}
		if !s.fleet {
			if detail != "" {
				detail += " · "
			}
			detail += fmt.Sprintf(i18n.T(i18n.MembersScore), member.Score)
		}
		if member.MedalName != "" || member.MedalLevel > 0 {
			if detail != "" {
				detail += " · "
			}
			detail += fmt.Sprintf(i18n.T(i18n.MembersMedal), selectionText(member.MedalName), member.MedalLevel)
		}
		if ansi.StringWidth(name+"  "+detail) <= width {
			rows = append(rows, m.theme.textStyle.Bold(true).Render(name)+"  "+m.theme.muted.Render(detail))
		} else {
			rows = append(rows,
				m.theme.textStyle.Bold(true).Render(ansi.Wrap(name, width, "")),
				m.theme.muted.Render(ansi.Wrap(detail, width, "")),
			)
		}
	}
	if visible == 0 {
		rows = append(rows, m.theme.hintText(i18n.T(empty), width))
	}
	return strings.TrimRight(strings.Join(rows, "\n"), "\n")
}

func (m *Model) roomMembersKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.closeRoomMembers()
		return nil
	case "r":
		return m.refreshRoomMembers()
	case "home":
		m.view.GotoTop()
		return nil
	case "end":
		m.view.GotoBottom()
		return nil
	}
	var cmd tea.Cmd
	m.view, cmd = m.view.Update(msg)
	return cmd
}
