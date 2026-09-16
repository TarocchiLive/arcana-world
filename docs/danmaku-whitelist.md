# 弹幕显示白名单

这份清单说明「弹幕」页会显示哪些消息。按 `2` 打开弹幕页，按 `f` 开关附加事件。

“始终显示”的消息不受 `f` 影响；“开启 `f` 才显示”的消息默认隐藏；“始终隐藏”的消息即使开启 `f` 也不会显示。未列出的消息同样隐藏，但收到的原始数据仍会保存。

“显示”是指消息会出现在 TUI 弹幕列表中。大航海指舰长、提督和总督；SC 是付费醒目留言；PK / 乱斗是主播之间的互动比赛。“新版、镜像、开放平台”只用于区分同类消息的发送方式，普通使用时无需区分这些名称。

每项先写中文含义，括号内名称仅供核对。复选框和“意见”用于记录审阅结果，不是软件开关。

## 始终显示：聊天、礼物与直播间通知（34 项）

- [ ] 直播被切断。（`CUT_OFF`）意见：
- [ ] 普通弹幕（表情弹幕保留文字内容；不同版本的普通弹幕也包含在内）。（`DANMU_MSG`）意见：
- [ ] 镜像弹幕。（`DANMU_MSG_MIRROR`）意见：
- [ ] 大航海购买。（`GUARD_BUY`）意见：
- [ ] 本房间的大航海提示（与跨房广播不同）。（`GUIARD_MSG`）意见：
- [ ] 观众点赞。（`LIKE_INFO_V3_CLICK`）意见：
- [ ] 直播开始。（`LIVE`）意见：
- [ ] 开放平台弹幕。（`LIVE_OPEN_PLATFORM_DM`）意见：
- [ ] 开放平台镜像弹幕。（`LIVE_OPEN_PLATFORM_DM_MIRROR`）意见：
- [ ] 开放平台大航海。（`LIVE_OPEN_PLATFORM_GUARD`）意见：
- [ ] 开放平台点赞。（`LIVE_OPEN_PLATFORM_LIKE`）意见：
- [ ] 开放平台下播。（`LIVE_OPEN_PLATFORM_LIVE_END`）意见：
- [ ] 开放平台入场。（`LIVE_OPEN_PLATFORM_LIVE_ROOM_ENTER`）意见：
- [ ] 开放平台开播。（`LIVE_OPEN_PLATFORM_LIVE_START`）意见：
- [ ] 开放平台礼物。（`LIVE_OPEN_PLATFORM_SEND_GIFT`）意见：
- [ ] 开放平台 SC。（`LIVE_OPEN_PLATFORM_SUPER_CHAT`）意见：
- [ ] 开放平台 SC 撤回。（`LIVE_OPEN_PLATFORM_SUPER_CHAT_DEL`）意见：
- [ ] 直播结束 / 准备中。（`PREPARING`）意见：
- [ ] 弹幕撤回通知。（`RECALL_DANMU_MSG`）意见：
- [ ] 撤销房管。（`ROOM_ADMIN_REVOKE`）意见：
- [ ] 观众被禁言。（`ROOM_BLOCK_MSG`）意见：
- [ ] 房间标题 / 分区信息更新。（`ROOM_CHANGE`）意见：
- [ ] 关闭房间禁言。（`ROOM_SILENT_OFF`）意见：
- [ ] 开启房间禁言。（`ROOM_SILENT_ON`）意见：
- [ ] 礼物。（`SEND_GIFT`）意见：
- [ ] 新版礼物消息。（`SEND_GIFT_V2`）意见：
- [ ] 醒目留言 SC。（`SUPER_CHAT_MESSAGE`）意见：
- [ ] SC 撤回。（`SUPER_CHAT_MESSAGE_DELETE`）意见：
- [ ] 日语版醒目留言（SC，保留原文，同一条不会重复显示）。（`SUPER_CHAT_MESSAGE_JPN`）意见：
- [ ] 大航海提示。（`USER_TOAST_MSG`）意见：
- [ ] 新版大航海提示。（`USER_TOAST_MSG_V2`）意见：
- [ ] 另一种大航海提示。（`USER_TOAST_V2`）意见：
- [ ] 直播违规警告。（`WARNING`）意见：
- [ ] 新增房管。（`room_admin_entrance`）意见：

## 观众互动：入场、关注与分享

这几种通知按内容区分，不是观众发送的普通弹幕。`INTERACT_WORD` 和 `INTERACT_WORD_V2` 都按下表决定是否显示：

- [ ] 观众互动通知。（`INTERACT_WORD`）意见：
- [ ] 新版观众互动通知。（`INTERACT_WORD_V2`）意见：

| 通知 | 什么时候显示 | 怎样理解 | 核对与意见 |
| --- | --- | --- | --- |
| 观众入场 | 始终显示 | 提示观众进入当前直播间；不等于完整在线名单，也不保证每次进房都有通知。 | [ ] 意见： |
| 关注 | 始终显示 | 当前直播间收到的关注通知，通常是观众关注主播的提示。 | [ ] 意见： |
| 分享 | 开启 `f` 才显示 | 当前直播间收到的分享通知；无法据此判断分享渠道或分享是否完成。 | [ ] 意见： |
| 特别关注 | 开启 `f` 才显示 | 通知名称是“特别关注”，但具体含义和出现条件还不确定，不能认定是点击某个按钮后的结果。 | [ ] 意见： |
| 互关 | 始终隐藏 | 名称表示“互相关注”，但还不确定是在提示刚刚互关，还是已有的互关关系，不能当作新增互关人数。 | [ ] 意见： |

其他尚未识别的观众互动通知也不显示。

## 开启 `f` 才显示：附加通知（62 项）

本节包括抽奖、入场特效、连麦、房间统计和 PK 进展；目前支持的 30 项 PK 通知也都列在这里，默认不显示。

- [ ] 天选时刻中奖结果。（`ANCHOR_LOT_AWARD`）意见：
- [ ] 天选时刻资格检查。（`ANCHOR_LOT_CHECKSTATUS`）意见：
- [ ] 天选时刻结束。（`ANCHOR_LOT_END`）意见：
- [ ] 天选时刻开始。（`ANCHOR_LOT_START`）意见：
- [ ] 礼物连击结束汇总。（`COMBO_END`）意见：
- [ ] 礼物连击汇总。（`COMBO_SEND`）意见：
- [ ] 礼物抽奖结果。（`DANMU_GIFT_LOTTERY_AWARD`）意见：
- [ ] 礼物抽奖结束。（`DANMU_GIFT_LOTTERY_END`）意见：
- [ ] 礼物抽奖开始。（`DANMU_GIFT_LOTTERY_START`）意见：
- [ ] 连续弹幕消息。（`DM_INTERACTION`）意见：
- [ ] 特效弹幕。（`EFFECT_DANMAKU_MSG`）意见：
- [ ] 入场特效。（`ENTRY_EFFECT`）意见：
- [ ] 本房累计点赞数。（`LIKE_INFO_V3_UPDATE`）意见：
- [ ] 再次匹配PK。（`PK_AGAIN`）意见：
- [ ] 乱斗入口状态。（`PK_BATTLE_ENTRANCE`）意见：
- [ ] 乱斗决胜阶段。（`PK_BATTLE_PRO_TYPE`）意见：
- [ ] 乱斗段位更新。（`PK_BATTLE_RANK_CHANGE`）意见：
- [ ] 乱斗票数加成。（`PK_BATTLE_VOTES_ADD`）意见：
- [ ] 再次PK请求。（`PK_CLICK_AGAIN`）意见：
- [ ] PK结束。（`PK_END`）意见：
- [ ] PK邀请取消。（`PK_INVITE_CANCEL`）意见：
- [ ] PK邀请失败。（`PK_INVITE_FAIL`）意见：
- [ ] PK邀请。（`PK_INVITE_INIT`）意见：
- [ ] PK邀请拒绝。（`PK_INVITE_REFUSE`）意见：
- [ ] PK邀请已关闭。（`PK_INVITE_SWITCH_CLOSE`）意见：
- [ ] PK邀请已开启。（`PK_INVITE_SWITCH_OPEN`）意见：
- [ ] PK抽奖开始。（`PK_LOTTERY_START`）意见：
- [ ] PK匹配。（`PK_MATCH`）意见：
- [ ] 连麦PK结束。（`PK_MIC_END`）意见：
- [ ] PK准备。（`PK_PRE`）意见：
- [ ] PK比分。（`PK_PROCESS`）意见：
- [ ] PK结算。（`PK_SETTLE`）意见：
- [ ] PK开始。（`PK_START`）意见：
- [ ] PK 暴击。（`PK_BATTLE_CRIT`）意见：
- [ ] 乱斗结束。（`PK_BATTLE_END`）意见：
- [ ] 乱斗礼物效果。（`PK_BATTLE_GIFT`）意见：
- [ ] PK 匹配超时。（`PK_BATTLE_MATCH_TIMEOUT`）意见：
- [ ] 乱斗准备。（`PK_BATTLE_PRE`）意见：
- [ ] 乱斗比分。（`PK_BATTLE_PROCESS`）意见：
- [ ] 乱斗结算。（`PK_BATTLE_SETTLE`）意见：
- [ ] 乱斗用户结算。（`PK_BATTLE_SETTLE_USER`）意见：
- [ ] PK 道具。（`PK_BATTLE_SPECIAL_GIFT`）意见：
- [ ] 乱斗开始。（`PK_BATTLE_START`）意见：
- [ ] 直播间红包。（`POPULARITY_RED_POCKET_NEW`）意见：
- [ ] 红包开始。（`POPULARITY_RED_POCKET_START`）意见：
- [ ] 红包开始（新版）。（`POPULARITY_RED_POCKET_V2_START`）意见：
- [ ] 红包中奖名单（新版）。（`POPULARITY_RED_POCKET_V2_WINNER_LIST`）意见：
- [ ] 红包中奖名单。（`POPULARITY_RED_POCKET_WINNER_LIST`）意见：
- [ ] 房管名单。（`ROOM_ADMINS`）意见：
- [ ] 房间粉丝统计更新。（`ROOM_REAL_TIME_MESSAGE_UPDATE`）意见：
- [ ] 房间粉丝统计更新（新版）。（`ROOM_REAL_TIME_MESSAGE_UPDATE_V2`）意见：
- [ ] 虚拟 MVP 解锁。（`USER_VIRTUAL_MVP`）意见：
- [ ] 结束视频连线。（`VIDEO_CONNECTION_JOIN_END`）意见：
- [ ] 邀请视频连线。（`VIDEO_CONNECTION_JOIN_START`）意见：
- [ ] 视频连线信息。（`VIDEO_CONNECTION_MSG`）意见：
- [ ] 连麦申请列表更新。（`VOICE_JOIN_LIST`）意见：
- [ ] 连麦申请人数。（`VOICE_JOIN_ROOM_COUNT_INFO`）意见：
- [ ] 语音连麦状态。（`VOICE_JOIN_STATUS`）意见：
- [ ] 连麦开关。（`VOICE_JOIN_SWITCH`）意见：
- [ ] 本房累计看过人数（不是实时在线人数）。（`WATCHED_CHANGE`）意见：
- [ ] 欢迎用户。（`WELCOME`）意见：
- [ ] 欢迎舰队成员。（`WELCOME_GUARD`）意见：

## 始终隐藏：不会出现在弹幕列表中（79 项）

以下消息即使开启 `f` 也不会出现在弹幕列表中。列出它们是为了说明显示范围，不表示这些活动或功能可在应用内操作。

- [ ] 活动横幅关闭。（`ACTIVITY_BANNER_CLOSE`）意见：
- [ ] 活动提醒。（`ACTIVITY_BANNER_RED_NOTICE`）意见：
- [ ] 活动提醒关闭。（`ACTIVITY_BANNER_RED_NOTICE_CLOSE`）意见：
- [ ] 活动横幅更新。（`ACTIVITY_BANNER_UPDATE`）意见：
- [ ] BLS 活动横幅更新。（`ACTIVITY_BANNER_UPDATE_BLS`）意见：
- [ ] 活动横幅更新。（`ACTIVITY_BANNER_UPDATE_V2`）意见：
- [ ] 赛事应援。（`ACTIVITY_MATCH_GIFT`）意见：
- [ ] 主播通知。（`ANCHOR_NORMAL_NOTIFY`）意见：
- [ ] 活动动画。（`ANIMATION`）意见：
- [ ] 直播间在所属分区的排名改变。（`AREA_RANK_CHANGED`）意见：
- [ ] 首领战攻击。（`BOSS_BATTLE`）意见：
- [ ] 首领战能量。（`BOSS_ENERGY`）意见：
- [ ] 首领战状态。（`BOSS_INFO`）意见：
- [ ] 首领战生命值。（`BOSS_INJURY`）意见：
- [ ] 宝箱活动开始。（`BOX_ACTIVITY_START`）意见：
- [ ] 房间背景更新。（`CHANGE_ROOM_INFO`）意见：
- [ ] 通用通知。（`COMMON_NOTICE_DANMAKU`）意见：
- [ ] 每日任务新一天。（`DAILY_QUEST_NEWDAY`）意见：
- [ ] 每日任务奖励。（`DAILY_QUEST_REWARD`）意见：
- [ ] 弹幕活动特效配置。（`DANMU_ACTIVITY_CONFIG`）意见：
- [ ] 其他直播间推荐卡片。（`FLOW_REWARD_CARD`）意见：
- [ ] 免费礼物提示。（`FREE_GIFT_BUBBLE`）意见：
- [ ] 全屏特效。（`FULL_SCREEN_SPECIAL_EFFECT`）意见：
- [ ] 礼物星球点亮。（`GIFT_STAR_PROCESS`）意见：
- [ ] 大航海成就。（`GUARD_ACHIEVEMENT_ROOM`）意见：
- [ ] 舰队抽奖开始。（`GUARD_LOTTERY_START`）意见：
- [ ] 跨房广播的总督上船消息（不是本房间的大航海提示）。（`GUARD_MSG`）意见：
- [ ] 直播间限时热门榜排名改变。（`HOT_RANK_CHANGED`）意见：
- [ ] 当前直播间限时热门榜排名改变V2。（`HOT_RANK_CHANGED_V2`）意见：
- [ ] 限时热门榜上榜信息。（`HOT_RANK_SETTLEMENT`）意见：
- [ ] 限时热门榜上榜信息V2。（`HOT_RANK_SETTLEMENT_V2`）意见：
- [ ] 热门房间通知。（`HOT_ROOM_NOTIFY`）意见：
- [ ] 小时榜奖励。（`HOUR_RANK_AWARDS`）意见：
- [ ] 点赞相关通知文案（普通点赞事件另行保留）。（`LIKE_INFO_V3_NOTICE`）意见：
- [ ] 勋章提示。（`LITTLE_TIPS`）意见：
- [ ] 互动消息推送结束。（`LIVE_OPEN_PLATFORM_INTERACTION_END`）意见：
- [ ] 英雄联盟赛事活动。（`LOL_ACTIVITY`）意见：
- [ ] 抽奖开始。（`LOTTERY_START`）意见：
- [ ] 主播幸运礼物奖励。（`LUCK_GIFT_AWARD_MASTER`）意见：
- [ ] 用户幸运礼物奖励。（`LUCK_GIFT_AWARD_USER`）意见：
- [ ] 粉丝勋章奖励通知。（`MESSAGEBOX_USER_GAIN_MEDAL`）意见：
- [ ] 通用公告 / 广播，可能来自其他房间。（`NOTICE_MSG`）意见：
- [ ] 移动端通知。（`NOTICE_MSG_H5`）意见：
- [ ] 高能榜人数（不是实时在线人数）。（`ONLINE_RANK_COUNT`）意见：
- [ ] 用户到达直播间高能榜前三名的消息。（`ONLINE_RANK_TOP3`）意见：
- [ ] 高能用户榜。（`ONLINE_RANK_V2`）意见：
- [ ] 新版高能用户榜。（`ONLINE_RANK_V3`）意见：
- [ ] 含义尚未确认的互动通知。（`PLAY_TOGETHER`）意见：
- [ ] 直播间在人气榜的排名改变。（`POPULAR_RANK_CHANGED`）意见：
- [ ] 礼物抽奖结果。（`RAFFLE_END`）意见：
- [ ] 礼物抽奖开始。（`RAFFLE_START`）意见：
- [ ] 榜单排名更新。（`RANK_CHANGED_V2`）意见：
- [ ] 含义尚未确认的房间通知。（`REENTER_LIVE_ROOM`）意见：
- [ ] 首领战奖励面板。（`ROOM_BOX_BOOS_AWARD`）意见：
- [ ] 主播宝箱奖励面板。（`ROOM_BOX_MASTER`）意见：
- [ ] 用户宝箱奖励面板。（`ROOM_BOX_USER`）意见：
- [ ] 房间限制。（`ROOM_LIMIT`）意见：
- [ ] 房间封禁。（`ROOM_LOCK`）意见：
- [ ] 房间排名。（`ROOM_RANK`）意见：
- [ ] 房间屏蔽规则。（`ROOM_SHIELD`）意见：
- [ ] 房间皮肤状态。（`ROOM_SKIN_MSG`）意见：
- [ ] 积分卡活动。（`SCORE_CARD`）意见：
- [ ] 节奏风暴状态。（`SPECIAL_GIFT`）意见：
- [ ] 下播房间列表，即使包含本房也不展示此列表。（`STOP_LIVE_ROOM_LIST`）意见：
- [ ] 醒目留言按钮。（`SUPER_CHAT_ENTRANCE`）意见：
- [ ] 系统礼物广播。（`SYS_GIFT`）意见：
- [ ] 系统广播。（`SYS_MSG`）意见：
- [ ] 小电视抽奖结果。（`TV_END`）意见：
- [ ] 小电视抽奖开始。（`TV_START`）意见：
- [ ] 获得粉丝勋章。（`USER_GAIN_MEDAL`）意见：
- [ ] 用户信息更新。（`USER_INFO_UPDATE`）意见：
- [ ] 用户面板提醒。（`USER_PANEL_RED_ALARM`）意见：
- [ ] 获得用户头衔。（`USER_TITLE_GET`）意见：
- [ ] 周星打卡。（`WEEK_STAR_CLOCK`）意见：
- [ ] 顶部横幅。（`WIDGET_BANNER`）意见：
- [ ] 礼物心愿单进度。（`WIDGET_WISH_LIST`）意见：
- [ ] 实物抽奖结束。（`WIN_ACTIVITY`）意见：
- [ ] 许愿瓶状态。（`WISH_BOTTLE`）意见：
- [ ] 新主播奖励。（`new_anchor_reward`）意见：

## 始终显示：连接状态（另计 3 项）

这些提示由应用记录，帮助你了解监听是否正常，不计入上面的 177 项消息。

- [ ] 开始连接。（`session_start`）意见：
- [ ] 监听已停止。（`session_end`）意见：
- [ ] 连接中断。（`connection_lost`）意见：
