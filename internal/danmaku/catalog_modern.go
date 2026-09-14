package danmaku

import "arcana-world/internal/i18n"

// Arrays retain their decoded members, including winners, awards and notice segments.
// Undocumented numeric values remain raw; opaque protobuf payloads are deliberately
// not treated as decoded details here (their dedicated decoders own those commands).
var modernCommandSpecs = map[string]commandSpec{
	"ENTRY_EFFECT": {i18n.DanmakuModernEntryEffect, []commandField{
		{"data.uid", 'i'}, {"data.uinfo.base.name", 's'}, {"data.target_id", 'i'},
		{"data.copy_writing", 's'}, {"data.copy_writing_v2", 's'}, {"data.privilege_type", 'n'},
		{"data.wealthy_info.level", 'n'}, {"data.effective_time", 'n'}, {"data.trigger_time", 'n'},
	}},
	"USER_VIRTUAL_MVP": {i18n.DanmakuModernVirtualMVP, []commandField{
		{"data.uid", 'i'}, {"data.uname", 's'}, {"data.action", 's'}, {"data.goods_id", 'i'},
		{"data.goods_name", 's'}, {"data.goods_num", 'n'}, {"data.goods_price", 'n'},
		{"data.order_id", 'i'}, {"data.success_toast", 's'}, {"data.user_guard_level", 'n'}, {"data.timestamp", 'n'},
	}},
	"POPULARITY_RED_POCKET_START": {i18n.DanmakuModernRedPocketStart, []commandField{
		{"data.lot_id", 'i'}, {"data.sender_uid", 'i'}, {"data.sender_name", 's'},
		{"data.danmu", 's'}, {"data.join_requirement", 'n'}, {"data.awards", 'a'},
		{"data.total_price", 'n'}, {"data.lot_status", 'n'}, {"data.user_status", 'n'},
		{"data.start_time", 'n'}, {"data.end_time", 'n'}, {"data.wait_num", 'n'},
	}},
	"POPULARITY_RED_POCKET_V2_START": {i18n.DanmakuModernRedPocketV2Start, []commandField{
		{"data.lot_id", 'i'}, {"data.sender_uid", 'i'}, {"data.sender_name", 's'},
		{"data.sender_uinfo.base.name", 's'}, {"data.is_mystery", 'b'}, {"data.rp_type", 'n'},
		{"data.danmu", 's'}, {"data.join_requirement", 'n'}, {"data.awards", 'a'},
		{"data.total_price", 'n'}, {"data.lot_status", 'n'}, {"data.user_status", 'n'},
		{"data.start_time", 'n'}, {"data.end_time", 'n'}, {"data.wait_num", 'n'}, {"data.wait_num_v2", 'n'},
	}},
	"POPULARITY_RED_POCKET_WINNER_LIST": {i18n.DanmakuModernRedPocketWinners, []commandField{
		{"data.lot_id", 'i'}, {"data.total_num", 'n'}, {"data.winner_info", 'a'}, {"data.awards", 'o'}, {"data.version", 'n'},
	}},
	"POPULARITY_RED_POCKET_V2_WINNER_LIST": {i18n.DanmakuModernRedPocketV2Winners, []commandField{
		{"data.lot_id", 'i'}, {"data.total_num", 'n'}, {"data.award_num", 'n'},
		{"data.winner_info", 'a'}, {"data.awards", 'o'}, {"data.rp_type", 'n'}, {"data.version", 'n'},
	}},
	"ANCHOR_LOT_START": {i18n.DanmakuModernLotteryStart, []commandField{
		{"data.id", 'i'}, {"data.room_id", 'i'}, {"data.award_name", 's'}, {"data.award_num", 'n'},
		{"data.award_price_text", 's'}, {"data.danmu", 's'}, {"data.danmu_new", 'a'},
		{"data.gift_name", 's'}, {"data.gift_num", 'n'}, {"data.gift_price", 'n'},
		{"data.require_text", 's'}, {"data.require_type", 'n'}, {"data.require_value", 'n'},
		{"data.join_type", 'n'}, {"data.lot_status", 'n'}, {"data.status", 'n'}, {"data.max_time", 'n'},
	}},
	"ANCHOR_LOT_AWARD": {i18n.DanmakuModernLotteryAward, []commandField{
		{"data.id", 'i'}, {"data.award_name", 's'}, {"data.award_num", 'n'},
		{"data.award_users", 'a'}, {"data.award_dont_popup", 'n'}, {"data.lot_status", 'n'},
	}},
	"COMMON_NOTICE_DANMAKU": {i18n.DanmakuModernCommonNotice, []commandField{
		{"data.content_segments", 'a'},
	}},
	"EFFECT_DANMAKU_MSG": {i18n.DanmakuModernEffectMessage, []commandField{
		{"data.sender_uinfo.uid", 'i'}, {"data.sender_uinfo.base.name", 's'},
		{"data.goods_info.action", 's'}, {"data.goods_info.name", 's'}, {"data.goods_info.num", 's'},
		{"data.goods_info.prefix", 's'}, {"data.goods_info.text", 's'}, {"msg_id", 's'}, {"send_time", 'n'},
	}},
	"DANMU_ACTIVITY_CONFIG": {i18n.DanmakuModernActivityConfig, []commandField{
		{"data.id", 'i'}, {"data.unique_id", 's'}, {"data.dm_mode", 'n'}, {"data.dm_setting_switch", 'n'},
		{"data.status", 'n'}, {"data.stime", 'n'}, {"data.etime", 'n'}, {"data.platform", 'a'},
		{"data.screen_type", 'n'}, {"data.material_conf", 'o'}, {"data.mock_options", 'a'},
	}},
	"ROOM_SILENT_ON": {i18n.DanmakuModernSilentOn, []commandField{
		{"data.type", 's'}, {"data.level", 'n'}, {"data.second", 'n'}, {"data.msg", 's'},
	}},
	// Older servers send data: []; the command itself fully identifies this state.
	"ROOM_SILENT_OFF": {i18n.DanmakuModernSilentOff, nil},
	"ONLINE_RANK_V2": {i18n.DanmakuModernOnlineRank, []commandField{
		{"data.online_list", 'a'}, {"data.rank_type", 's'},
	}},
	"ROOM_REAL_TIME_MESSAGE_UPDATE": {i18n.DanmakuModernRoomStats, []commandField{
		{"data.roomid", 'i'}, {"data.fans", 'n'}, {"data.fans_club", 'n'},
	}},
	"ROOM_REAL_TIME_MESSAGE_UPDATE_V2": {i18n.DanmakuModernRoomStatsV2, []commandField{
		{"data.roomid", 'i'}, {"data.fans", 'n'}, {"data.fans_club", 'n'},
	}},
	"WARNING": {i18n.DanmakuModernWarning, []commandField{
		{"msg", 's'}, {"roomid", 'i'},
	}},
	"ROOM_ADMINS": {i18n.DanmakuModernRoomAdmins, []commandField{
		{"uids", 'a'},
	}},
	"room_admin_entrance": {i18n.DanmakuModernAdminAdded, []commandField{
		{"uid", 'i'}, {"msg", 's'},
	}},
	"ROOM_ADMIN_REVOKE": {i18n.DanmakuModernAdminRevoked, []commandField{
		{"uid", 'i'}, {"msg", 's'},
	}},
}

func modernCommandSpec(cmd string) (commandSpec, bool) {
	spec, ok := modernCommandSpecs[cmd]
	return spec, ok
}
