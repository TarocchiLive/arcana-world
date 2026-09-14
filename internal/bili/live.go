// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
	"arcana-world/internal/i18n"

	"context"
	"errors"
	"net/http"
	"strings"

	"arcana-world/internal/domain"
)

const preLiveInfo = "/xlive/app-blink/v1/preLive/UpdatePreLiveInfo"

func (c *Client) Room(ctx context.Context) (domain.Room, error) {
	var room domain.Room
	if err := c.call(ctx, http.MethodGet, c.LiveBase, "/xlive/app-blink/v1/index/GetRoomPreLiveStatus", signed(nil, "access_key"), nil, false, false, nil); err != nil {
		return room, err
	}
	var pre struct {
		Title string `json:"title"`
		Cover struct {
			URL    string `json:"url"`
			Status *int   `json:"auditStatus"`
			Reason string `json:"auditReason"`
		} `json:"cover"`
	}
	p := signed(values("area", "true", "cover", "true", "coverVertical", "true", "liveDirectionType", "0", "mobi_app", "pc_link", "schedule", "true", "title", "true"))
	if err := c.call(ctx, http.MethodGet, c.LiveBase, "/xlive/app-blink/v1/preLive/PreLive", p, nil, false, false, &pre); err != nil {
		return room, err
	}
	var info struct {
		ID     integer `json:"room_id"`
		Parent string  `json:"parent_name"`
		Area   string  `json:"area_v2_name"`
		AreaID integer `json:"area_v2_id"`
		Live   *int    `json:"live_status"`
	}
	uid := c.account.Cookies["DedeUserID"]
	if uid == "" {
		return room, errors.New(i18n.T(i18n.BiliSignInRequired))
	}
	if err := c.call(ctx, http.MethodGet, c.LiveBase, "/xlive/app-blink/v1/room/GetInfo", signed(values("uId", uid)), nil, false, false, &info); err != nil {
		return room, err
	}
	if info.ID <= 0 || info.Live == nil || pre.Cover.Status == nil {
		return room, errors.New(i18n.T(i18n.BiliRoomResponseIncomplete))
	}
	var announcement struct {
		Announces map[string]struct {
			Content string `json:"content"`
		} `json:"announces"`
	}
	if err := c.call(ctx, http.MethodGet, c.LiveBase, "/xlive/app-blink/v1/room/AnnounceInfo", signed(nil), nil, false, false, &announcement); err != nil {
		return room, err
	}
	status := i18n.T(i18n.BiliCoverReviewUnknown)
	switch *pre.Cover.Status {
	case -1:
		status = i18n.T(i18n.BiliCoverRejected)
	case 0:
		status = i18n.T(i18n.BiliCoverUnderReview)
	case 1:
		status = i18n.T(i18n.BiliCoverApproved)
	}
	room = domain.Room{ID: int64(info.ID), Title: pre.Title, AreaID: int64(info.AreaID), AreaName: info.Area, ParentName: info.Parent, Live: *info.Live == 1, CoverURL: pre.Cover.URL, CoverStatus: status, Announcement: announcement.Announces["1"].Content}
	c.roomID = room.ID
	return room, nil
}
func (c *Client) Areas(ctx context.Context) ([]domain.Area, error) {
	var d struct {
		Parents []struct {
			Name string `json:"name"`
			List []struct {
				ID   integer `json:"id"`
				Name string  `json:"name"`
			} `json:"list"`
		} `json:"area_v1_info"`
	}
	if err := c.call(ctx, http.MethodGet, c.LiveBase, "/xlive/app-blink/v1/preLive/GetAreaListForLive", signed(nil), nil, false, false, &d); err != nil {
		return nil, err
	}
	if d.Parents == nil {
		return nil, errors.New(i18n.T(i18n.BiliCategoriesIncomplete))
	}
	areas := make([]domain.Area, 0)
	for _, parent := range d.Parents {
		for _, area := range parent.List {
			if area.ID <= 0 || area.Name == "" || parent.Name == "" {
				return nil, errors.New(i18n.T(i18n.BiliCategoryInvalid))
			}
			areas = append(areas, domain.Area{ID: int64(area.ID), Name: area.Name, Parent: parent.Name})
		}
	}
	return areas, nil
}
func (c *Client) Start(ctx context.Context, roomID, areaID int64, protocol string) (domain.Stream, error) {
	if roomID <= 0 || areaID <= 0 {
		return domain.Stream{}, errors.New(i18n.T(i18n.BiliRoomCategoryInvalid))
	}
	if protocol != "rtmp" && protocol != "srt" && protocol != "srt-fallback" {
		return domain.Stream{}, errors.New(i18n.T(i18n.BiliStreamProtocolInvalid))
	}
	if err := c.ensureDevice(ctx); err != nil {
		return domain.Stream{}, err
	}
	p, err := c.csrf(values("room_id", decimal(roomID), "area_v2", decimal(areaID), "type", "2"))
	if err != nil {
		return domain.Stream{}, err
	}
	p = signed(p)
	e, _, err := c.request(ctx, http.MethodPost, c.LiveBase, "/room/v1/Room/startLive", nil, strings.NewReader(encode(p)), "application/x-www-form-urlencoded", false, false)
	if err != nil {
		return domain.Stream{}, err
	}
	c.roomID = roomID
	if *e.Code == 60024 || *e.Code == 60043 {
		var d struct {
			QR   string `json:"qr"`
			Risk struct {
				Voucher string `json:"v_voucher"`
			} `json:"risk_extra"`
		}
		if err = decode(e.Data, &d); err != nil {
			return domain.Stream{}, err
		}
		challenge := &domain.FaceChallenge{URL: d.QR, Voucher: d.Risk.Voucher, Message: i18n.T(i18n.BiliIdentityVerificationPrompt)}
		if *e.Code == 60024 && !validLink(challenge.URL) {
			return domain.Stream{}, errors.New(i18n.T(i18n.BiliIdentityLinkInvalid))
		}
		if *e.Code == 60043 && challenge.Voucher == "" {
			return domain.Stream{}, errors.New(i18n.T(i18n.BiliIdentityVoucherMissing))
		}
		return domain.Stream{}, challenge
	}
	if err = e.check(i18n.T(i18n.BiliStartLiveRoom)); err != nil {
		return domain.Stream{}, err
	}
	var d struct {
		RTMP struct {
			Addr string `json:"addr"`
			Code string `json:"code"`
		} `json:"rtmp"`
		Protocols []struct {
			Protocol string `json:"protocol"`
			Addr     string `json:"addr"`
			Code     string `json:"code"`
		} `json:"protocols"`
	}
	if err = decode(e.Data, &d); err != nil {
		return domain.Stream{}, err
	}
	if protocol != "rtmp" {
		for _, p := range d.Protocols {
			if strings.EqualFold(p.Protocol, "srt") && p.Addr != "" && p.Code != "" {
				return domain.Stream{Address: p.Addr, Key: p.Code, Protocol: "srt"}, nil
			}
		}
	}
	if protocol == "srt" {
		return domain.Stream{}, errors.New(i18n.T(i18n.BiliSrtUrlMissing))
	}
	if d.RTMP.Addr == "" || d.RTMP.Code == "" {
		return domain.Stream{}, errors.New(i18n.T(i18n.BiliStreamUrlMissing))
	}
	result := domain.Stream{Address: d.RTMP.Addr, Key: d.RTMP.Code, Protocol: "rtmp"}
	if protocol == "srt-fallback" {
		result.Warning = i18n.T(i18n.BiliSrtFallback)
	}
	return result, nil
}
func (c *Client) Stop(ctx context.Context, roomID int64) error {
	if roomID <= 0 {
		return errors.New(i18n.T(i18n.BiliRoomIdInvalid))
	}
	p, err := c.csrf(signed(values("room_id", decimal(roomID))))
	if err != nil {
		return err
	}
	return c.call(ctx, http.MethodPost, c.LiveBase, "/room/v1/Room/stopLive", nil, p, false, false, nil)
}
func (c *Client) SetTitle(ctx context.Context, roomID int64, title string) error {
	if roomID <= 0 || strings.TrimSpace(title) == "" {
		return errors.New(i18n.T(i18n.BiliRoomTitleInvalid))
	}
	p, err := c.csrf(values("mobi_app", "pc_link", "room_id", decimal(roomID), "title", title))
	if err != nil {
		return err
	}
	return c.call(ctx, http.MethodPost, c.LiveBase, preLiveInfo, signed(nil), p, false, false, nil)
}
func (c *Client) SetArea(ctx context.Context, roomID, areaID int64) error {
	if roomID <= 0 || areaID <= 0 {
		return errors.New(i18n.T(i18n.BiliRoomCategoryInvalid))
	}
	p, err := c.csrf(values("area_id", decimal(areaID), "build", build, "platform", "pc_link", "room_id", decimal(roomID)))
	if err != nil {
		return err
	}
	return c.call(ctx, http.MethodPost, c.LiveBase, "/xlive/app-blink/v2/room/AnchorChangeRoomArea", signed(nil), p, false, false, nil)
}
func (c *Client) SetAnnouncement(ctx context.Context, roomID int64, content string) error {
	if roomID <= 0 {
		return errors.New(i18n.T(i18n.BiliRoomIdInvalid))
	}
	// AnnounceCommit 使用已认证的主播身份，而不是 room_id 字段。
	p, err := c.csrf(signed(nil))
	if err != nil {
		return err
	}
	p.Set("content", content)
	p.Set("type", "1")
	return c.call(ctx, http.MethodPost, c.LiveBase, "/xlive/app-blink/v1/room/AnnounceCommit", nil, p, false, false, nil)
}
func (c *Client) timeShift(ctx context.Context) (int, int, int, error) {
	if c.roomID == 0 {
		if _, err := c.Room(ctx); err != nil {
			return 0, 0, 0, err
		}
	}
	p, err := c.csrf(values("room_id", decimal(c.roomID)))
	if err != nil {
		return 0, 0, 0, err
	}
	var d struct {
		Shift *int `json:"time_shift"`
		Min   *int `json:"min_time_shift"`
		Max   *int `json:"max_time_shift"`
	}
	if err = c.call(ctx, http.MethodGet, c.LiveBase, "/xlive/app-blink/v1/upStreamConfig/GetAnchorSelfStreamTimeShift", signed(p), nil, false, false, &d); err != nil {
		return 0, 0, 0, err
	}
	if d.Shift == nil || d.Min == nil || d.Max == nil || *d.Min > *d.Max {
		return 0, 0, 0, errors.New(i18n.T(i18n.BiliStreamDelayResponseInvalid))
	}
	return *d.Shift, *d.Min, *d.Max, nil
}
func (c *Client) TimeShift(ctx context.Context) (int, error) {
	shift, _, _, err := c.timeShift(ctx)
	return shift, err
}
func (c *Client) SetTimeShift(ctx context.Context, shift int) error {
	_, min, max, err := c.timeShift(ctx)
	if err != nil {
		return err
	}
	if shift < min || shift > max {
		return errors.New(i18n.T(i18n.BiliStreamDelayOutOfRange))
	}
	p, err := c.csrf(values("room_id", decimal(c.roomID), "time_shift", decimal(int64(shift))))
	if err != nil {
		return err
	}
	return c.call(ctx, http.MethodPost, c.LiveBase, "/xlive/app-blink/v1/upStreamConfig/SetAnchorSelfStreamTimeShift", nil, signed(p), false, false, nil)
}
