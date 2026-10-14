package tui

import (
	"errors"
	"fmt"
	"strconv"

	"arcana-world/internal/bili"
	"arcana-world/internal/i18n"
	"arcana-world/internal/termimage"
	"arcana-world/internal/webui"
	tea "charm.land/bubbletea/v2"
)

// 将目标身份及列表内容纳入版本，避免相同表单标题掩盖目标变化。
type webChatInteraction struct {
	client     *bili.Client
	account    string
	roomID     int64
	speaker    *chatSpeaker
	members    *roomMembersState
	moderation *moderationState
	user       bili.RoomUser
	users      *bili.RoomUser
	member     *bili.RoomMember
	query      string
	action     int
	hours      int64
	pending    bool
	loaded     bool
	loading    bool
}

func (c webChatInteraction) dialogIdentity() webChatInteraction {
	// 名单刷新状态与首项地址只使命令版本失效，不改变同一名单弹层身份。
	c.member, c.loaded, c.loading = nil, false, false
	return c
}

func (m *Model) webChatContext() webChatInteraction {
	c := webChatInteraction{client: m.client, account: m.config.ActiveUID, speaker: m.speaker, members: m.members, moderation: m.moderation}
	if m.room != nil {
		c.roomID = m.room.ID
	}
	if s := m.speaker; s != nil {
		c.user.UID, c.action, c.hours, c.pending = s.uid, s.action, s.hours, s.pending
	}
	if s := m.members; s != nil {
		c.loaded, c.loading = s.loaded, s.loading
		if len(s.list.Members) != 0 {
			c.member = &s.list.Members[0]
		}
	}
	if s := m.moderation; s != nil {
		c.user, c.action, c.hours, c.pending, c.query = s.user, int(s.action), s.hours, s.pending, s.query
		if len(s.users) != 0 {
			c.users = &s.users[0]
		}
	}
	return c
}

func (m *Model) webChatValid() bool {
	if m.speaker != nil {
		return m.speakerValid(m.speaker)
	}
	if m.members != nil {
		return m.roomMembersValid(m.members)
	}
	return m.moderationValid(m.moderation)
}

func (m *Model) clearWebChat() {
	if m.speaker != nil {
		m.speaker.pending = false
		m.closeChatSpeaker()
	}
	if m.members != nil {
		m.closeRoomMembers()
	}
	if m.moderation != nil {
		m.moderation.pending = false
		m.view = m.moderation.parentView
		m.moderation = nil
	}
	m.confirmAction = ""
	m.editKind = ""
	m.choices = nil
}

func (m *Model) webChatBackAvailable() bool {
	return m.webChatValid() && (m.mode == "pick" || m.mode == "form") && m.editKind != "moderation-menu"
}

func (m *Model) webSpeakerAction(action string) (tea.Cmd, error) {
	index := -1
	switch action {
	case "speaker-mute":
		index = 0
	case "speaker-block":
		index = 1
	case "speaker-admin":
		index = 2
	}
	// 浏览器主页仅使用 DTO 的 HTTPS 链接，不调用终端的 OS 打开器。
	if index < 0 || !m.speakerValid(m.speaker) {
		return nil, errors.New(i18n.T(i18n.WebActionUnavailable))
	}
	m.speaker.selected = index
	return m.chooseChatSpeaker(), nil
}

func (m *Model) webChatAction(c webui.Command) (tea.Cmd, error) {
	invalid := func() (tea.Cmd, error) { return nil, errors.New(i18n.T(i18n.WebActionUnavailable)) }
	if m.client == nil || m.account == nil || m.account.UID != m.config.ActiveUID || m.room == nil {
		return invalid()
	}
	switch c.Action {
	case "audience", "fleet":
		if m.chat == nil || m.members != nil {
			return invalid()
		}
		return m.openRoomMembers(c.Action == "fleet"), nil
	case "moderation-admins":
		return m.openModeration(moderationAdmins), nil
	case "moderation-blocks":
		return m.openModeration(moderationBlocks), nil
	case "moderation-mutes":
		return m.openModeration(moderationMutes), nil
	case "speaker-open", "speaker-mute", "speaker-block", "speaker-admin":
		if m.chat == nil || m.chat.state.AccountUID != m.account.UID {
			return invalid()
		}
		for _, event := range m.chat.entries {
			if event.RoomID != m.room.ID || fmt.Sprintf("%d:%d:%d", event.RoomID, event.Sequence, event.Time.UnixNano()) != c.Value {
				continue
			}
			avatar := m.openChatSpeaker(event)
			if !m.speakerValid(m.speaker) {
				return invalid()
			}
			if c.Action == "speaker-open" {
				return avatar, nil
			}
			next, err := m.webSpeakerAction(c.Action)
			return tea.Batch(avatar, next), err
		}
	}
	return invalid()
}

func (m *Model) webAvatar(avatar *termimage.Thumbnail, fallback string) string {
	if avatar == nil {
		return webCoverURL(fallback)
	}
	if m.web.imageAvatar != avatar || m.web.image == "" {
		m.web.imageAvatar = avatar
		m.web.image = avatar.DataURL()
	}
	return m.web.image
}

func (m *Model) webChatDialog(d *webui.Interaction) bool {
	if m.mode == "confirm" {
		if m.moderationValid(m.moderation) {
			d.Image = m.webAvatar(m.moderation.avatar, m.moderation.user.Face)
		} else if m.speakerValid(m.speaker) {
			d.Image = m.webAvatar(m.speaker.avatar, m.speaker.event.Face)
		}
	}
	if m.mode == "members" && m.roomMembersValid(m.members) {
		s := m.members
		d.Kind = "members"
		title := i18n.MembersTitle
		if s.fleet {
			title = i18n.FleetTitle
		}
		d.Title = i18n.T(title)
		if s.err != nil {
			d.Text = m.webText(s.err.Error())
		}
		d.Members = &webui.Members{Total: s.list.Total, Loaded: s.loaded, Loading: s.loading, Previous: s.loaded && s.err != nil, Entries: make([]webui.Member, 0, len(s.list.Members))}
		for _, member := range s.list.Members {
			entry := webui.Member{Rank: member.Rank, Name: m.webText(member.Name), MedalName: m.webText(member.MedalName), MedalLevel: member.MedalLevel, Mystery: member.Mystery}
			if !member.Mystery && member.UID > 0 {
				entry.UID = strconv.FormatInt(member.UID, 10)
			}
			switch member.GuardLevel {
			case 1:
				entry.Role = i18n.T(i18n.MembersGovernor)
			case 2:
				entry.Role = i18n.T(i18n.MembersAdmiral)
			case 3:
				entry.Role = i18n.T(i18n.MembersCaptain)
			}
			if !s.fleet {
				entry.Score = fmt.Sprintf(i18n.T(i18n.MembersScore), member.Score)
			}
			d.Members.Entries = append(d.Members.Entries, entry)
		}
		return true
	}
	if m.mode == "speaker" && m.speakerValid(m.speaker) {
		s := m.speaker
		d.Kind = "speaker"
		d.Title = m.webText(s.event.User)
		medal := i18n.T(i18n.SpeakerMedalUnknown)
		if s.event.MedalName != "" && s.event.MedalLevel > 0 {
			medal = fmt.Sprintf("%s · Lv.%d", m.webText(s.event.MedalName), s.event.MedalLevel)
		}
		d.Text = fmt.Sprintf("%s\nUID %d\n%s", d.Title, s.uid, medal)
		if s.result != "" {
			d.Text += "\n" + m.webText(s.result)
		}
		d.Image = m.webAvatar(s.avatar, s.event.Face)
		d.URL = "https://space.bilibili.com/" + strconv.FormatInt(s.uid, 10)
		for index, action := range []string{"speaker-mute", "speaker-block", "speaker-admin"} {
			d.Choices = append(d.Choices, webui.Option{ID: action, Label: i18n.T(speakerActionLabel(index))})
		}
		return true
	}
	return false
}
