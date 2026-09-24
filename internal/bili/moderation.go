// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"arcana-world/internal/i18n"
)

// RoomUser 表示可核对身份的房间用户。
type RoomUser struct {
	UID  int64
	Name string
	Face string
}

// SearchRoomUsers 使用直播中心的用户搜索，不依赖全站搜索的 WBI 签名。
// 此接口按名称或 UID 返回候选 items，官方调用没有分页参数。
func (c *Client) SearchRoomUsers(ctx context.Context, name string) ([]RoomUser, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New(i18n.T(i18n.BiliModerationInputInvalid))
	}
	var result struct {
		Items []struct {
			UID  integer `json:"uid"`
			Name string  `json:"uname"`
			Face string  `json:"face"`
		} `json:"items"`
	}
	if err := c.call(ctx, http.MethodGet, c.LiveBase, "/banned_service/v2/Silent/search_user", values("search", name), nil, false, true, &result); err != nil {
		return nil, err
	}
	if result.Items == nil {
		return nil, errors.New(i18n.T(i18n.BiliResponseDataInvalid))
	}
	users := make([]RoomUser, 0, len(result.Items))
	for _, item := range result.Items {
		if item.UID <= 0 || item.Name == "" {
			return nil, errors.New(i18n.T(i18n.BiliResponseDataInvalid))
		}
		users = append(users, RoomUser{UID: int64(item.UID), Name: item.Name, Face: moderationFace(item.Face)})
	}
	return users, nil
}

// RoomAdmins 分页读取当前主播的房管；平台按登录账号而非 room_id 确定作用域。
func (c *Client) RoomAdmins(ctx context.Context, roomID int64) ([]RoomUser, error) {
	if err := c.moderationOwnRoom(ctx, roomID); err != nil {
		return nil, err
	}
	var users []RoomUser
	seen := make(map[int64]bool)
	for page := int64(1); ; page++ {
		var result struct {
			Users []struct {
				UID  integer `json:"uid"`
				Name string  `json:"uname"`
				Face string  `json:"face"`
			} `json:"data"`
			Page struct {
				Total *integer `json:"total_page"`
			} `json:"page"`
		}
		if err := c.call(ctx, http.MethodGet, c.LiveBase, "/xlive/app-ucenter/v1/roomAdmin/get_by_anchor", values("page", decimal(page)), nil, false, true, &result); err != nil {
			return nil, err
		}
		if result.Page.Total == nil || *result.Page.Total < 0 {
			return nil, errors.New(i18n.T(i18n.BiliResponseDataInvalid))
		}
		added := 0
		for _, item := range result.Users {
			uid := int64(item.UID)
			if uid <= 0 || item.Name == "" {
				return nil, errors.New(i18n.T(i18n.BiliResponseDataInvalid))
			}
			if !seen[uid] {
				seen[uid] = true
				users = append(users, RoomUser{UID: uid, Name: item.Name, Face: moderationFace(item.Face)})
				added++
			}
		}
		if page >= int64(*result.Page.Total) {
			return users, nil
		}
		if added == 0 {
			return nil, errors.New(i18n.T(i18n.BiliResponseDataInvalid))
		}
	}
}

// RoomBlocks 读取房间禁言名单，不读取或修改账号黑名单。
func (c *Client) RoomBlocks(ctx context.Context, roomID int64) ([]RoomUser, error) {
	if roomID <= 0 {
		return nil, errors.New(i18n.T(i18n.BiliModerationInputInvalid))
	}
	var users []RoomUser
	seen := make(map[int64]bool)
	for page := int64(1); ; page++ {
		var result struct {
			Users []struct {
				UID  integer `json:"tuid"`
				Name string  `json:"tname"`
				Face string  `json:"face"`
			} `json:"data"`
			Total *integer `json:"total_page"`
		}
		if err := c.moderationPost(ctx, "/xlive/web-ucenter/v1/banned/GetSilentUserList", values("room_id", decimal(roomID), "ps", decimal(page)), &result); err != nil {
			return nil, err
		}
		if result.Total == nil || *result.Total < 0 {
			return nil, errors.New(i18n.T(i18n.BiliResponseDataInvalid))
		}
		added := 0
		for _, item := range result.Users {
			uid := int64(item.UID)
			if uid <= 0 || item.Name == "" {
				return nil, errors.New(i18n.T(i18n.BiliResponseDataInvalid))
			}
			if !seen[uid] {
				seen[uid] = true
				users = append(users, RoomUser{UID: uid, Name: item.Name, Face: moderationFace(item.Face)})
				added++
			}
		}
		if page >= int64(*result.Total) {
			return users, nil
		}
		if added == 0 {
			return nil, errors.New(i18n.T(i18n.BiliResponseDataInvalid))
		}
	}
}

// AddRoomAdmin 仅任命普通房管，不授予高级房管的账号拉黑权限。
func (c *Client) AddRoomAdmin(ctx context.Context, roomID, uid int64) error {
	if uid <= 0 {
		return errors.New(i18n.T(i18n.BiliModerationInputInvalid))
	}
	if err := c.moderationOwnRoom(ctx, roomID); err != nil {
		return err
	}
	return c.moderationPost(ctx, "/xlive/web-ucenter/v1/roomAdmin/appoint", values("admin", decimal(uid), "admin_level", "1"), nil)
}

func (c *Client) RemoveRoomAdmin(ctx context.Context, roomID, uid int64) error {
	if uid <= 0 {
		return errors.New(i18n.T(i18n.BiliModerationInputInvalid))
	}
	if err := c.moderationOwnRoom(ctx, roomID); err != nil {
		return err
	}
	return c.moderationPost(ctx, "/xlive/app-ucenter/v1/roomAdmin/dismiss", values("uid", decimal(uid)), nil)
}

// AddRoomBlock 永久禁止用户在此直播间发言，不影响账号关注关系或其他直播间。
func (c *Client) AddRoomBlock(ctx context.Context, roomID, uid int64) error {
	return c.muteRoomUser(ctx, roomID, uid, "-1")
}

// MuteRoomUser 禁止用户在此直播间发言两小时。
func (c *Client) MuteRoomUser(ctx context.Context, roomID, uid int64) error {
	return c.muteRoomUser(ctx, roomID, uid, "2")
}

func (c *Client) muteRoomUser(ctx context.Context, roomID, uid int64, hour string) error {
	if roomID <= 0 || uid <= 0 {
		return errors.New(i18n.T(i18n.BiliModerationInputInvalid))
	}
	return c.moderationPost(ctx, "/xlive/web-ucenter/v1/banned/AddSilentUser", values("room_id", decimal(roomID), "tuid", decimal(uid), "mobile_app", "web", "hour", hour), nil)
}

func (c *Client) RemoveRoomBlock(ctx context.Context, roomID, uid int64) error {
	if roomID <= 0 || uid <= 0 {
		return errors.New(i18n.T(i18n.BiliModerationInputInvalid))
	}
	// 当前直播中心使用目标 UID 撤销禁言，无需依赖旧接口的记录编号。
	return c.moderationPost(ctx, "/xlive/web-ucenter/v1/banned/DelSilentUser", values("room_id", decimal(roomID), "tuid", decimal(uid)), nil)
}

func (c *Client) moderationPost(ctx context.Context, path string, form url.Values, out any) error {
	form, err := c.csrf(form)
	if err != nil {
		return err
	}
	return c.call(ctx, http.MethodPost, c.LiveBase, path, nil, form, false, true, out)
}

// 房管接口不接受房间参数，先核对主播自己的房间，避免误操作其他作用域。
func (c *Client) moderationOwnRoom(ctx context.Context, roomID int64) error {
	if roomID <= 0 {
		return errors.New(i18n.T(i18n.BiliModerationInputInvalid))
	}
	uid := c.account.Cookies["DedeUserID"]
	if uid == "" || c.account.Cookies["SESSDATA"] == "" {
		return errors.New(i18n.T(i18n.BiliSignInRequired))
	}
	var room struct {
		ID integer `json:"room_id"`
	}
	if err := c.call(ctx, http.MethodGet, c.LiveBase, "/xlive/app-blink/v1/room/GetInfo", signed(values("uId", uid)), nil, false, false, &room); err != nil {
		return err
	}
	if int64(room.ID) != roomID {
		return errors.New(i18n.T(i18n.BiliModerationOwnRoomRequired))
	}
	return nil
}

func moderationFace(face string) string {
	if strings.HasPrefix(face, "//") {
		return "https:" + face
	}
	if strings.HasPrefix(face, "http://") {
		return "https://" + strings.TrimPrefix(face, "http://")
	}
	return face
}
