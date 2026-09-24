package tui

import (
	"context"
	"fmt"
	"time"

	"arcana-world/internal/bili"
	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
)

const audienceInterval = 30 * time.Second

type audienceUI struct {
	client         *bili.Client
	roomID         int64
	uid            string
	generation     uint64
	count          int64
	known, loading bool
	next           time.Time
}

type audienceMsg struct {
	client     *bili.Client
	roomID     int64
	uid        string
	generation uint64
	count      int64
	err        error
}

func (m *Model) audienceIdentity() (int64, string) {
	if m.room == nil || m.account == nil {
		return 0, ""
	}
	return m.room.ID, m.account.UID
}

func (m *Model) updateAudience() tea.Cmd {
	roomID, uid := m.audienceIdentity()
	a := &m.audience
	if a.client != m.client || a.roomID != roomID || a.uid != uid {
		*a = audienceUI{client: m.client, roomID: roomID, uid: uid, generation: a.generation + 1}
	}
	if roomID <= 0 || uid == "" || m.client == nil || m.session == nil || a.loading || time.Now().Before(a.next) {
		return nil
	}
	a.loading = true
	generation := a.generation
	client, session, ctx := m.client, m.session, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		msg := audienceMsg{client: client, roomID: roomID, uid: uid, generation: generation}
		if msg.err = session.Lock(ctx); msg.err == nil {
			request := client.AudienceRequest(roomID)
			session.Unlock()
			msg.count, msg.err = request(ctx)
		}
		return msg
	}
}

func (m *Model) applyAudience(msg audienceMsg) {
	roomID, uid := m.audienceIdentity()
	if msg.client != m.client || msg.roomID != roomID || msg.uid != uid || msg.generation != m.audience.generation {
		return
	}
	m.audience = audienceUI{client: msg.client, roomID: roomID, uid: uid, generation: msg.generation, count: msg.count, known: msg.err == nil, next: time.Now().Add(audienceInterval)}
}

func (m *Model) audienceText() string {
	roomID, uid := m.audienceIdentity()
	a := m.audience
	if a.known && a.client == m.client && a.roomID == roomID && a.uid == uid {
		return fmt.Sprintf(i18n.T(i18n.TUIAudienceCount), m.visibleAudienceCount(a.count))
	}
	return i18n.T(i18n.TUIAudienceUnknown)
}

func (m *Model) visibleAudienceCount(count int64) int64 {
	// 接口进房会计入当前账号；在线总数独立于仅返回前 20 名的榜单。
	if !m.config.AudienceIncludeSelf && m.account != nil && m.account.UID != "" {
		return max(0, count-1)
	}
	return count
}
