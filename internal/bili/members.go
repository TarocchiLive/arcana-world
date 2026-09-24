package bili

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"arcana-world/internal/i18n"
)

const RoomMemberLimit = 20

type RoomMember struct {
	UID        int64
	Name       string
	Rank       int64
	GuardLevel int
	MedalName  string
	MedalLevel int64
	Score      int64
	Mystery    bool
	// 在匿名 UID 脱敏前识别当前账号，不向展示层暴露匿名身份。
	Self bool
}

type RoomMemberList struct {
	Total   int64
	Members []RoomMember
}

// RoomMembers 读取当前直播间成员榜第一页，在线人数不等于返回的榜单人数。
func (c *Client) RoomMembers(ctx context.Context, roomID int64) (RoomMemberList, error) {
	incomplete := func() (RoomMemberList, error) {
		return RoomMemberList{}, errors.New(i18n.T(i18n.BiliRoomResponseIncomplete))
	}
	uid := c.account.UID
	if uid == "" {
		uid = c.account.Cookies["DedeUserID"]
	}
	accountUID, err := strconv.ParseInt(uid, 10, 64)
	if roomID <= 0 || err != nil || accountUID <= 0 {
		return incomplete()
	}
	type member struct {
		UID        integer `json:"uid"`
		Name       string  `json:"name"`
		Rank       integer `json:"userRank"`
		Score      integer `json:"score"`
		GuardLevel integer `json:"guard_level"`
		Mystery    bool    `json:"is_mystery"`
		Medal      struct {
			Name  string  `json:"medalName"`
			Level integer `json:"level"`
		} `json:"medalInfo"`
	}
	var data struct {
		Total *integer  `json:"onlineNum"`
		List  *[]member `json:"OnlineRankItem"`
	}
	if err := c.call(ctx, http.MethodGet, c.LiveBase, "/xlive/general-interface/v1/rank/getOnlineGoldRank", values("roomId", decimal(roomID), "ruid", uid, "page", "1", "pageSize", strconv.Itoa(RoomMemberLimit)), nil, false, true, &data); err != nil {
		return RoomMemberList{}, err
	}
	if data.Total == nil || *data.Total < 0 || data.List == nil {
		return incomplete()
	}
	result := RoomMemberList{Total: int64(*data.Total), Members: make([]RoomMember, 0, len(*data.List))}
	for _, item := range *data.List {
		if (!item.Mystery && item.UID <= 0) || item.Name == "" || item.Rank <= 0 || item.Score < 0 || item.GuardLevel < 0 || item.Medal.Level < 0 {
			return incomplete()
		}
		level := 0
		if item.GuardLevel <= 3 {
			level = int(item.GuardLevel)
		}
		uid := int64(item.UID)
		if item.Mystery {
			// 匿名成员只使用接口公开的名称，不读取原始身份，也不向调用方暴露 UID。
			uid = 0
		}
		result.Members = append(result.Members, RoomMember{
			UID: uid, Name: item.Name, Rank: int64(item.Rank), GuardLevel: level,
			MedalName: item.Medal.Name, MedalLevel: int64(item.Medal.Level),
			Score: int64(item.Score), Mystery: item.Mystery, Self: int64(item.UID) == accountUID,
		})
	}
	result.Members = normalizeRoomMembers(result.Members)
	return result, nil
}

// RoomFleet 读取大航海榜第一页，榜单成员不代表当前在线观众。
func (c *Client) RoomFleet(ctx context.Context, roomID int64) (RoomMemberList, error) {
	incomplete := func() (RoomMemberList, error) {
		return RoomMemberList{}, errors.New(i18n.T(i18n.BiliRoomResponseIncomplete))
	}
	uid := c.account.UID
	if uid == "" {
		uid = c.account.Cookies["DedeUserID"]
	}
	accountUID, err := strconv.ParseInt(uid, 10, 64)
	if roomID <= 0 || err != nil || accountUID <= 0 {
		return incomplete()
	}
	type member struct {
		UID        integer `json:"uid"`
		Name       string  `json:"username"`
		Rank       integer `json:"rank"`
		GuardLevel integer `json:"guard_level"`
		Medal      struct {
			Name  string  `json:"medal_name"`
			Level integer `json:"medal_level"`
		} `json:"medal_info"`
	}
	var data struct {
		Info struct {
			Total *integer `json:"num"`
		} `json:"info"`
		Top3 []member `json:"top3"`
		List []member `json:"list"`
	}
	if err := c.call(ctx, http.MethodGet, c.LiveBase, "/xlive/app-room/v2/guardTab/topList", values("roomid", decimal(roomID), "ruid", uid, "page", "1", "page_size", strconv.Itoa(RoomMemberLimit)), nil, false, true, &data); err != nil {
		return RoomMemberList{}, err
	}
	if data.Info.Total == nil || *data.Info.Total < 0 {
		return incomplete()
	}
	result := RoomMemberList{Total: int64(*data.Info.Total), Members: make([]RoomMember, 0, len(data.Top3)+len(data.List))}
	for _, group := range [][]member{data.Top3, data.List} {
		for _, item := range group {
			if item.UID <= 0 || item.Name == "" || item.Rank <= 0 || item.GuardLevel < 0 || item.Medal.Level < 0 {
				return incomplete()
			}
			level := 0
			if item.GuardLevel <= 3 {
				level = int(item.GuardLevel)
			}
			result.Members = append(result.Members, RoomMember{
				UID: int64(item.UID), Name: item.Name, Rank: int64(item.Rank), GuardLevel: level,
				MedalName: item.Medal.Name, MedalLevel: int64(item.Medal.Level),
			})
		}
	}
	if (result.Total == 0) != (len(result.Members) == 0) {
		return incomplete()
	}
	result.Members = normalizeRoomMembers(result.Members)
	return result, nil
}

func normalizeRoomMembers(members []RoomMember) []RoomMember {
	// 按真实名次排序再去重，重叠成员保留更靠前的名次；同名次保留原有顺序。
	sort.SliceStable(members, func(i, j int) bool { return members[i].Rank < members[j].Rank })
	seen := make(map[int64]struct{}, RoomMemberLimit)
	result := members[:0]
	for _, item := range members {
		// 匿名成员没有可用于去重的公开 UID，不合并不同匿名名次。
		if !item.Mystery {
			if _, exists := seen[item.UID]; exists {
				continue
			}
			seen[item.UID] = struct{}{}
		}
		result = append(result, item)
		if len(result) == RoomMemberLimit {
			break
		}
	}
	return result
}
