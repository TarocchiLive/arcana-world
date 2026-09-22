package app

import (
	"arcana-world/internal/bili"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"context"
	"errors"
)

// AccountOutcome 保留刚获取的旧直播间状态，即使清理或提交失败。
type AccountOutcome struct {
	Account domain.Account
	Client  *bili.Client
	OldRoom *domain.Room
}

func (m *Session) client(source *bili.Client, account *domain.Account, proxy string) (*bili.Client, error) {
	c, err := bili.New(proxy)
	if err != nil {
		return nil, err
	}
	c.APIBase, c.LiveBase, c.PassportBase = source.APIBase, source.LiveBase, source.PassportBase
	if account != nil {
		c.SetAccount(*account)
	}
	return c, nil
}

// detach 查询已提交的身份，绝不使用可能过期的 UI 直播间状态。
func (m *Session) detach(ctx context.Context, source *bili.Client, out *AccountOutcome) error {
	cfg := m.store.Config()
	if cfg.ActiveUID == "" {
		return nil
	}
	old, err := m.store.Load(cfg.ActiveUID)
	if err != nil {
		return err
	}
	c, err := m.client(source, &old, m.store.Config().Proxy)
	if err != nil {
		return err
	}
	defer c.HTTP.CloseIdleConnections()
	room, err := c.Room(ctx)
	if err != nil {
		return err
	}
	out.OldRoom = &room
	if _, err = m.stopControlledOBS(ctx, cfg, room.Live); err != nil {
		return err
	}
	if room.Live {
		if err = c.Stop(ctx, room.ID); err != nil {
			return err
		}
		room.Live = false
	}
	return nil
}

// Switch 先验证，再清理旧身份，最后提交 ActiveUID。
// 提交前取消会保留旧身份；已确认的远端操作结果
// 不受后续错误影响，始终返回。提交成功后不会回滚。
func (m *Session) Switch(ctx context.Context, source *bili.Client, uid string, login *domain.Account) (AccountOutcome, error) {
	out := AccountOutcome{}
	if err := m.Lock(ctx); err != nil {
		return out, err
	}
	defer m.Unlock()
	var a domain.Account
	var err error
	if login != nil {
		a = *login
	} else {
		a, err = m.store.Load(uid)
		if err != nil {
			return out, err
		}
	}
	c, err := m.client(source, &a, m.store.Config().Proxy)
	if err != nil {
		return out, err
	}
	committed := false
	defer func() {
		if !committed {
			c.HTTP.CloseIdleConnections()
		}
	}()
	info, err := c.Validate(ctx)
	if err != nil {
		return out, err
	}
	if (a.UID != "" && info.UID != a.UID) || (a.Cookies["DedeUserID"] != "" && info.UID != a.Cookies["DedeUserID"]) {
		return out, errors.New(i18n.T(i18n.TUIErrorAccountMismatch))
	}
	a.UID, a.Name = info.UID, info.Name
	cfg := m.store.Config()
	if cfg.ActiveUID != "" && cfg.ActiveUID != a.UID {
		if err = m.detach(ctx, source, &out); err != nil {
			return out, err
		}
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if m.closing.Load() {
		return out, context.Canceled
	}
	if err = m.store.Save(a); err != nil {
		return out, err
	}
	cfg = m.store.Config()
	cfg.ActiveUID = a.UID
	if err = m.store.SaveConfig(cfg); err != nil {
		return out, err
	}
	committed = true
	m.Track(c)
	out.Account, out.Client = a, c
	return out, nil
}

func (m *Session) Delete(ctx context.Context, source *bili.Client, uid string) (AccountOutcome, error) {
	out := AccountOutcome{}
	if err := m.Lock(ctx); err != nil {
		return out, err
	}
	defer m.Unlock()
	active := m.store.Config().ActiveUID == uid
	if active {
		if err := m.detach(ctx, source, &out); err != nil {
			return out, err
		}
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if m.closing.Load() {
		return out, context.Canceled
	}
	var c *bili.Client
	if active {
		var err error
		c, err = m.client(source, nil, m.store.Config().Proxy)
		if err != nil {
			return out, err
		}
	}
	if err := m.store.Delete(uid); err != nil {
		if c != nil {
			c.HTTP.CloseIdleConnections()
		}
		return out, err
	}
	if c != nil {
		m.Track(c)
		out.Client = c
	}
	return out, nil
}
