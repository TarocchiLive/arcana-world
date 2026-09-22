package tui

import (
	"strings"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
)

// 同时展示协议原始值与已知含义。
// 未知取值及没有公开枚举定义的字段保持原样。
func chatEnumText(command string, field danmaku.EventField) string {
	var label i18n.Key
	switch strings.TrimPrefix(field.Name, "data.") {
	case "guard_level", "privilege_type", "user_guard_level":
		switch field.Value {
		case "0":
			label = i18n.DanmakuGuardNone
		case "1":
			label = i18n.DanmakuGuardGovernor
		case "2":
			label = i18n.DanmakuGuardAdmiral
		case "3":
			label = i18n.DanmakuGuardCaptain
		}
	case "type":
		if command == "ROOM_SILENT_ON" {
			switch field.Value {
			case "all":
				label = i18n.DanmakuMuteAll
			case "level":
				label = i18n.DanmakuMuteLevel
			case "wealth":
				label = i18n.DanmakuMuteWealth
			case "medal":
				label = i18n.DanmakuMuteMedal
			case "member":
				label = i18n.DanmakuMuteMember
			case "follow":
				label = i18n.DanmakuMuteFollow
			}
		}
	case "second":
		if command == "ROOM_SILENT_ON" && field.Value == "-1" {
			label = i18n.DanmakuPermanent
		}
	case "rp_type":
		if command == "POPULARITY_RED_POCKET_V2_START" || command == "POPULARITY_RED_POCKET_V2_WINNER_LIST" {
			switch field.Value {
			case "1":
				label = i18n.DanmakuPocketGift
			case "2":
				label = i18n.DanmakuPocketGuard
			case "3":
				label = i18n.DanmakuPocketBattery
			}
		}
	}
	text := chatText(field.Value, 384)
	if label != "" {
		text += "(" + i18n.T(label) + ")"
	}
	return text
}
