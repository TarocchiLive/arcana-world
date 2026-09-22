package danmaku

import "arcana-world/internal/i18n"

// 开放平台线格式参考：https://github.com/xfgryujk/blivedm/blob/dev/blivedm/models/open_live.py。
// 已归档的 API-collect message_stream.md 记载了旧版 toast 结构。
// 其他归档样例：EricEcho/BilibiliTool/doc/danmu-cmd.md；
// RANK_CHANGED_V2 字段：urlynn/blivemsg/src/internal/parser.rs。
var coreCommandSpecs = map[string]commandSpec{
	"RECALL_DANMU_MSG":                   {title: i18n.DanmakuRecallNotice, fields: []commandField{{"data.target_id", 'i'}, {"data.recall_type", 'n'}}},
	"USER_TOAST_MSG":                     {title: i18n.DanmakuGuardToast, fields: []commandField{{"data.username", 's'}, {"data.role_name", 's'}, {"data.guard_level", 'n'}, {"data.num", 'n'}, {"data.unit", 's'}, {"data.price", 'n'}, {"data.op_type", 'n'}, {"data.toast_msg", 's'}}},
	"LIVE_OPEN_PLATFORM_SEND_GIFT":       {title: i18n.DanmakuOpenGift, fields: []commandField{{"data.uname", 's'}, {"data.gift_name", 's'}, {"data.gift_num", 'n'}, {"data.price", 'n'}, {"data.r_price", 'n'}, {"data.paid", 'b'}, {"data.combo_info", 'o'}, {"data.blind_gift", 'o'}}},
	"LIVE_OPEN_PLATFORM_GUARD":           {title: i18n.DanmakuOpenGuard, fields: []commandField{{"data.user_info.uname", 's'}, {"data.guard_level", 'n'}, {"data.guard_num", 'n'}, {"data.guard_unit", 's'}, {"data.price", 'n'}}},
	"LIVE_OPEN_PLATFORM_INTERACTION_END": {title: i18n.DanmakuOpenInteractionEnd, fields: []commandField{{"data.game_id", 's'}, {"data.timestamp", 'n'}}},
	"DANMU_GIFT_LOTTERY_START": {title: i18n.DanmakuCoreGiftLotteryStart, fields: []commandField{
		{"data.id", 'i'}, {"data.room_id", 'i'}, {"data.title", 's'}, {"data.award_name", 's'}, {"data.award_num", 'n'},
		{"data.gift_name", 's'}, {"data.cur_gift_num", 'n'}, {"data.price", 'n'}, {"data.danmu", 's'}, {"data.require_text", 's'},
		{"data.join_type", 'n'}, {"data.require_type", 'n'}, {"data.require_value", 'n'}, {"data.status", 'n'}, {"data.time", 'n'},
	}},
	"DANMU_GIFT_LOTTERY_END": {title: i18n.DanmakuCoreGiftLotteryEnd, fields: []commandField{{"data.id", 'i'}, {"data.room_id", 'i'}}},
	"DANMU_GIFT_LOTTERY_AWARD": {title: i18n.DanmakuCoreGiftLotteryAward, fields: []commandField{
		{"data.id", 'i'}, {"data.award_id", 'i'}, {"data.award_name", 's'}, {"data.award_num", 'n'}, {"data.award_text", 's'}, {"data.award_type", 'n'}, {"data.award_users", 'a'},
	}},
	"GUARD_ACHIEVEMENT_ROOM": {title: i18n.DanmakuCoreGuardAchievement, fields: []commandField{
		{"data.room_id", 'i'}, {"data.first_line_content", 's'}, {"data.second_line_content", 's'},
		{"data.current_achievement_level", 'n'}, {"data.event_type", 'n'}, {"data.is_first", 'b'}, {"data.show_time", 'n'},
	}},
	"PK_BATTLE_CRIT": {title: i18n.DanmakuCorePKCrit, fields: []commandField{
		{"pk_id", 'i'}, {"pk_status", 'n'}, {"timestamp", 'n'}, {"data.accept_crit_room_id", 'i'},
		{"data.send_uid", 'i'}, {"data.send_uname", 's'}, {"data.gift_name", 's'}, {"data.gift_num", 'n'}, {"data.crit_num", 'n'}, {"data.pk_votes_name", 's'},
	}},
	"WEEK_STAR_CLOCK": {title: i18n.DanmakuCoreWeekStarClock, fields: []commandField{
		{"data.id", 'i'}, {"data.activity_title", 's'}, {"data.title", 's'}, {"data.title_text", 's'},
		{"data.rank", 's'}, {"data.rank_name", 's'}, {"data.expire_hour", 'n'}, {"data.type", 'n'}, {"data.is_close", 'n'},
		{"data.is_clock_in_over", 'n'}, {"data.gift_progress", 'a'}, {"web_data.rank", 's'}, {"web_data.rank_name", 's'}, {"web_data.gift_progress", 'o'},
	}},
	"VOICE_JOIN_STATUS": {title: i18n.DanmakuCoreVoiceStatus, fields: []commandField{
		{"data.room_id", 'i'}, {"data.status", 'n'}, {"data.channel", 's'}, {"data.channel_type", 's'},
		{"data.uid", 'i'}, {"data.user_name", 's'}, {"data.guard", 'n'}, {"data.start_at", 'n'}, {"data.current_time", 'n'},
	}},
	"RANK_CHANGED_V2":         {title: i18n.DanmakuCoreRankChanged, fields: []commandField{{"data.rank_name_by_type", 's'}, {"data.rank", 'n'}, {"data.on_rank_name_by_type", 's'}}},
	"PK_BATTLE_MATCH_TIMEOUT": {title: i18n.DanmakuCorePKTimeout},
	"PK_BATTLE_SPECIAL_GIFT": {title: i18n.DanmakuCorePKSpecialGift, fields: []commandField{
		{"pk_id", 'i'}, {"pk_status", 'n'}, {"data.type", 'n'}, {"data.room_id", 'i'}, {"data.gift_name", 's'}, {"data.gift_num", 'n'}, {"data.send_uid", 'i'}, {"data.send_uname", 's'},
	}},
	"VOICE_JOIN_LIST": {title: i18n.DanmakuCoreVoiceList, fields: []commandField{
		{"data.room_id", 'i'}, {"data.category", 'n'}, {"data.apply_count", 'n'}, {"data.red_point", 'n'}, {"data.refresh", 'n'},
	}},
	"VOICE_JOIN_ROOM_COUNT_INFO": {title: i18n.DanmakuCoreVoiceCount, fields: []commandField{
		{"data.room_id", 'i'}, {"data.root_status", 'n'}, {"data.room_status", 'n'}, {"data.apply_count", 'n'}, {"data.notify_count", 'n'}, {"data.red_point", 'n'},
	}},
	"VOICE_JOIN_SWITCH": {title: i18n.DanmakuCoreVoiceSwitch, fields: []commandField{{"data.room_id", 'i'}, {"data.root_status", 'n'}, {"data.room_status", 'n'}}},
	"GUIARD_MSG":        {title: i18n.DanmakuGuardToast, fields: []commandField{{"msg", 's'}, {"buy_type", 'n'}}},
	"ACTIVITY_BANNER_UPDATE_V2": {title: i18n.DanmakuCoreBannerV2, fields: []commandField{
		{"data.id", 'i'}, {"data.title", 's'}, {"data.banner_type", 'n'}, {"data.closeable", 'n'}, {"data.add_banner", 'n'}, {"data.weight", 'n'}, {"data.jump_url", 's'},
	}},
	"ACTIVITY_BANNER_UPDATE_BLS": {title: i18n.DanmakuCoreBannerBLS, fields: []commandField{
		{"data.id", 'i'}, {"data.action", 's'}, {"data.gift_progress", 'a'}, {"data.is_close", 'n'}, {"data.weight", 'n'}, {"data.jump_url", 's'},
	}},
	"ANCHOR_NORMAL_NOTIFY": {title: i18n.DanmakuCoreAnchorNotify, fields: []commandField{
		{"data.info.title", 's'}, {"data.info.content", 's'}, {"data.type", 'n'}, {"data.show_type", 'n'},
	}},
	"HOT_ROOM_NOTIFY": {title: i18n.DanmakuCoreHotRoomNotify, fields: []commandField{{"data.ttl", 'n'}, {"data.exit_no_refresh", 'n'}, {"data.random_delay_req", 'a'}}},
}

func projectAdditionalCoreEvent(p projection, cmd string, root map[string]any) projection {
	if spec, ok := coreCommandSpecs[cmd]; ok {
		// 撤回通知的 target_id 没有明确约定为逐条消息的 ID。仅显示
		// 通知，不据此推断应删除某用户的所有消息。
		return projectCommandSpec(p, cmd, root, spec)
	}
	data := object(root["data"])
	switch cmd {
	case "LIVE_OPEN_PLATFORM_DM", "LIVE_OPEN_PLATFORM_DM_MIRROR":
		message, ok := data["msg"].(string)
		user, userOK := data["uname"].(string)
		if !ok || !userOK {
			return p
		}
		p.event.Kind, p.event.Text, p.event.User = "chat", message, user
		p.event.UID = stringValue(data["open_id"])
		if id := strong(data["msg_id"]); id != "" {
			p.identity = identity("open-chat", id)
		}
	case "LIVE_OPEN_PLATFORM_SUPER_CHAT":
		message, ok := data["message"].(string)
		user, userOK := data["uname"].(string)
		amount, amountOK := roomEventInteger(data["rmb"], false)
		id, idOK := commandFieldText(data["message_id"], 'i')
		if !ok || !userOK || !amountOK || !idOK || id == "0" {
			return p
		}
		p.event.Kind, p.event.Text, p.event.User = "sc", message, user
		p.event.UID, p.event.Amount = stringValue(data["open_id"]), amount
		p.scID = "open:" + id
		p.identity = identity("sc", p.scID)
	case "LIVE_OPEN_PLATFORM_SUPER_CHAT_DEL":
		ids, ok := data["message_ids"].([]any)
		if !ok {
			return p
		}
		deleted := make([]string, 0, len(ids))
		for _, value := range ids {
			id, valid := commandFieldText(value, 'i')
			if !valid || id == "0" {
				return p
			}
			deleted = append(deleted, "open:"+id)
		}
		p.event.Kind, p.event.Text, p.deletes = "delete", "", deleted
	case "LIVE_OPEN_PLATFORM_LIKE", "LIVE_OPEN_PLATFORM_LIVE_ROOM_ENTER":
		user, ok := data["uname"].(string)
		if !ok {
			return p
		}
		p.event.Kind, p.event.Text, p.event.User, p.event.UID = "like", "", user, stringValue(data["open_id"])
		if cmd == "LIVE_OPEN_PLATFORM_LIVE_ROOM_ENTER" {
			p.event.Kind = "enter"
		}
	case "LIVE_OPEN_PLATFORM_LIVE_START", "LIVE_OPEN_PLATFORM_LIVE_END":
		room, ok := roomEventInteger(data["room_id"], true)
		if !ok || room <= 0 {
			return p
		}
		p.event.Kind, p.event.Text = "live", ""
		if cmd == "LIVE_OPEN_PLATFORM_LIVE_END" {
			p.event.Kind = "preparing"
		}
	}
	return p
}
