package bili

import (
	"context"
	"errors"
	"net/http"

	"arcana-world/internal/i18n"
)

// AudienceRequest 在调用方的账号锁内取得只读快照，返回的请求可在解锁后执行。
// Cookie 必须深拷贝：登录刷新会原地更新票据；HTTP 仅共享可并发使用的 Transport。
func (c *Client) AudienceRequest(roomID int64) func(context.Context) (int64, error) {
	snapshot := Client{LiveBase: c.LiveBase, buvid: c.buvid}
	snapshot.SetAccount(c.account)
	if c.HTTP != nil {
		httpClient := *c.HTTP
		snapshot.HTTP = &httpClient
	}
	return func(ctx context.Context) (int64, error) {
		return snapshot.RoomAudience(ctx, roomID)
	}
}

// RoomAudience 读取在线榜提供的房间人数，不使用人气或累计看过人数。
func (c *Client) RoomAudience(ctx context.Context, roomID int64) (int64, error) {
	var data struct {
		Online *integer `json:"onlineNum"`
	}
	uid := c.account.UID
	if uid == "" {
		uid = c.account.Cookies["DedeUserID"]
	}
	if roomID <= 0 || uid == "" {
		return 0, errors.New(i18n.T(i18n.BiliRoomResponseIncomplete))
	}
	if err := c.call(ctx, http.MethodGet, c.LiveBase, "/xlive/general-interface/v1/rank/getOnlineGoldRank", values("roomId", decimal(roomID), "ruid", uid, "page", "1", "pageSize", "1"), nil, false, true, &data); err != nil {
		return 0, err
	}
	if data.Online == nil || *data.Online < 0 {
		return 0, errors.New(i18n.T(i18n.BiliRoomResponseIncomplete))
	}
	return int64(*data.Online), nil
}
