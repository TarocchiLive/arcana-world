package bili

import (
	"arcana-world/internal/i18n"

	"context"
	"errors"
	"net/http"

	"arcana-world/internal/domain"
)

// RecentAreas 将服务器仅包含名称的历史记录与当前分区目录匹配。
// 过时或有歧义的记录会被忽略，绝不为其猜测 ID。
func (c *Client) RecentAreas(ctx context.Context, roomID int64, all []domain.Area) ([]domain.Area, error) {
	if roomID <= 0 {
		return nil, errors.New(i18n.T(i18n.BiliRoomIdInvalid))
	}
	var entries []struct {
		ID     integer `json:"id"`
		Parent string  `json:"parent_name"`
		Name   string  `json:"name"`
	}
	if err := c.call(ctx, http.MethodGet, c.LiveBase, "/room/v1/Area/getMyChooseArea", signed(values("roomid", decimal(roomID))), nil, false, false, &entries); err != nil {
		return nil, err
	}
	type areaName struct{ parent, name string }
	byID := make(map[int64]domain.Area, len(all))
	byName := make(map[areaName]domain.Area, len(all))
	ambiguous := make(map[areaName]bool)
	for _, area := range all {
		if area.ID <= 0 {
			continue
		}
		byID[area.ID] = area
		key := areaName{area.Parent, area.Name}
		if previous, ok := byName[key]; ok && previous.ID != area.ID {
			ambiguous[key] = true
		}
		byName[key] = area
	}
	result := make([]domain.Area, 0, len(entries))
	seen := make(map[int64]bool)
	for _, entry := range entries {
		var area domain.Area
		var ok bool
		if entry.ID > 0 {
			area, ok = byID[int64(entry.ID)]
		} else {
			key := areaName{entry.Parent, entry.Name}
			area, ok = byName[key]
			ok = ok && !ambiguous[key]
		}
		if ok && !seen[area.ID] {
			seen[area.ID] = true
			result = append(result, area)
		}
	}
	return result, nil
}
