// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
	"context"
	"encoding/json"
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

// RoomMutes 分页读取房间禁言名单，与直播黑名单独立。
func (c *Client) RoomMutes(ctx context.Context, roomID int64) ([]RoomUser, error) {
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

// RoomBlocks 按主播 UID 分页读取真实直播黑名单；官方以空页结束分页。
func (c *Client) RoomBlocks(ctx context.Context, roomID int64) ([]RoomUser, error) {
	anchorID, err := c.moderationAnchor(ctx, roomID)
	if err != nil {
		return nil, err
	}
	var users []RoomUser
	seen := make(map[int64]bool)
	for page := int64(1); ; page++ {
		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := c.call(ctx, http.MethodGet, c.LiveBase, "/xlive/app-ucenter/v2/xbanned/banned/GetBlackList", values("anchor_id", decimal(anchorID), "pn", decimal(page), "ps", "10"), nil, false, true, &result); err != nil {
			return nil, err
		}
		var items []struct {
			UID  integer `json:"uid"`
			Name string  `json:"name"`
			Face string  `json:"face"`
		}
		if len(result.Data) == 0 || json.Unmarshal(result.Data, &items) != nil {
			return nil, errors.New(i18n.T(i18n.BiliResponseDataInvalid))
		}
		if len(items) == 0 {
			return users, nil
		}
		added := 0
		for _, item := range items {
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

// AddRoomBlock 将用户加入主播的直播黑名单，不使用永久禁言代替。
func (c *Client) AddRoomBlock(ctx context.Context, roomID, uid int64) error {
	if uid <= 0 {
		return errors.New(i18n.T(i18n.BiliModerationInputInvalid))
	}
	anchorID, err := c.moderationAnchor(ctx, roomID)
	if err != nil {
		return err
	}
	return c.moderationPost(ctx, "/xlive/app-ucenter/v2/xbanned/banned/AddBlack", values("anchor_id", decimal(anchorID), "tuid", decimal(uid), "spmid", "444.8.0.0"), nil)
}

// MuteRoomUser 按正整数小时禁言，-1 表示永久；与拉黑严格分开。
func (c *Client) MuteRoomUser(ctx context.Context, roomID, uid, hours int64) error {
	if roomID <= 0 || uid <= 0 || (hours <= 0 && hours != -1) {
		return errors.New(i18n.T(i18n.BiliModerationInputInvalid))
	}
	accountUID := c.account.UID
	if accountUID == "" {
		accountUID = c.account.Cookies["DedeUserID"]
	}
	if accountUID == decimal(uid) {
		return errors.New(i18n.T(i18n.BiliMuteSelfForbidden))
	}
	return c.moderationPost(ctx, "/xlive/web-ucenter/v1/banned/AddSilentUser", values("room_id", decimal(roomID), "tuid", decimal(uid), "mobile_app", "web", "type", "1", "hour", decimal(hours)), nil)
}

// RemoveRoomBlock 移出直播黑名单，不修改房间禁言。
func (c *Client) RemoveRoomBlock(ctx context.Context, roomID, uid int64) error {
	if uid <= 0 {
		return errors.New(i18n.T(i18n.BiliModerationInputInvalid))
	}
	anchorID, err := c.moderationAnchor(ctx, roomID)
	if err != nil {
		return err
	}
	return c.moderationPost(ctx, "/xlive/app-ucenter/v2/xbanned/banned/DelBlack", values("anchor_id", decimal(anchorID), "tuid", decimal(uid), "spmid", "444.8.0.0"), nil)
}

// RemoveRoomMute 撤销房间禁言，不修改直播黑名单。
func (c *Client) RemoveRoomMute(ctx context.Context, roomID, uid int64) error {
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

// 黑名单的作用域是房间主播，而非当前登录用户；同时支持短房间号。
func (c *Client) moderationAnchor(ctx context.Context, roomID int64) (int64, error) {
	if roomID <= 0 {
		return 0, errors.New(i18n.T(i18n.BiliModerationInputInvalid))
	}
	var room struct {
		UID     integer `json:"uid"`
		ID      integer `json:"room_id"`
		ShortID integer `json:"short_id"`
	}
	if err := c.call(ctx, http.MethodGet, c.LiveBase, "/room/v1/Room/get_info", values("room_id", decimal(roomID)), nil, false, true, &room); err != nil {
		return 0, err
	}
	if room.UID <= 0 || room.ID <= 0 || (int64(room.ID) != roomID && int64(room.ShortID) != roomID) {
		return 0, errors.New(i18n.T(i18n.BiliResponseDataInvalid))
	}
	return int64(room.UID), nil
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
