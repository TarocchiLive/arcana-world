// Package presentation 定义浮层与语音共享的事件目录及各自的中文模板。
package presentation

import (
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
	"github.com/charmbracelet/x/ansi"
)

type EventSpec struct {
	ID                           string
	Label                        i18n.Key
	OverlayTemplate, TTSTemplate string
}

// 仅收录聊天页默认显示的事件；详情事件以原始命令作为稳定标识。
var specs = []EventSpec{
	{"chat", i18n.OutputEventChat, `{{.User}}：{{.Text}}`, `{{.User}}说：{{.Text}}`},
	{"gift", i18n.OutputEventGift, `{{.User}}赠送{{.Gift}}{{if gt .Count 0}} × {{.Count}}{{end}}{{.Coins}}`, `感谢{{.User}}赠送{{if gt .Count 0}}{{.Count}}个{{end}}{{.Gift}}`},
	{"sc", i18n.OutputEventSC, `{{.User}}的醒目留言{{if gt .Amount 0}}（{{.Amount}}元）{{end}}：{{.Text}}`, `{{.User}}发送了{{if gt .Amount 0}}{{.Amount}}元的{{end}}醒目留言：{{.Text}}`},
	{"guard", i18n.OutputEventGuard, `{{.User}}开通了{{with .GuardDuration}}{{.}}的{{end}}{{.Gift}}`, `感谢{{.User}}开通了{{with .GuardDuration}}{{.}}的{{end}}{{.Gift}}`},
	{"enter", i18n.OutputEventEnter, `欢迎{{.User}}进入直播间`, `欢迎{{.User}}来到直播间`},
	{"follow", i18n.OutputEventFollow, `{{.User}}关注了主播`, `感谢{{.User}}的关注`},
	{"like", i18n.OutputEventLike, `{{.User}}点赞了直播间`, `感谢{{.User}}的点赞`},
	{"live", i18n.OutputEventLive, `直播已开始`, `主播开播了`},
	{"preparing", i18n.OutputEventPreparing, `直播已结束`, `本次直播已结束`},
	{"room_change", i18n.OutputEventRoomChange, `直播间标题已更新：{{.Text}}`, `直播间的新标题是：{{.Text}}`},
	{"room_block", i18n.OutputEventRoomBlock, `{{.User}}已被禁言`, `{{.User}}已被直播间禁言`},
	{"cut_off", i18n.OutputEventCutOff, `直播已被切断：{{.Text}}`, `直播已被切断。{{.Text}}`},
	{"delete", i18n.OutputEventDelete, `醒目留言已删除`, `有醒目留言被删除`},
	{"USER_TOAST_MSG", i18n.OutputEventGuardPurchase, `{{.F "data.username" "观众"}}开通了{{with .GuardDuration}}{{.}}的{{end}}{{.Guard}}`, `感谢{{.F "data.username" "观众"}}开通了{{with .GuardDuration}}{{.}}的{{end}}{{.Guard}}`},
	{"GUIARD_MSG", i18n.OutputEventGuardNotice, `大航海通知：{{.F "msg" "有人开通了大航海"}}`, `收到大航海通知。{{.F "msg" "有人开通了大航海"}}`},
	{"LIVE_OPEN_PLATFORM_SEND_GIFT", i18n.OutputEventInteractiveGift, `{{.F "data.uname" "观众"}}赠送{{.F "data.gift_name" "礼物"}}{{with .N "data.gift_num"}} × {{.}}{{end}}`, `感谢{{.F "data.uname" "观众"}}赠送{{with .N "data.gift_num"}}{{.}}个{{end}}{{.F "data.gift_name" "礼物"}}`},
	{"LIVE_OPEN_PLATFORM_GUARD", i18n.OutputEventInteractiveGuard, `{{.F "data.user_info.uname" "观众"}}开通了{{with .GuardDuration}}{{.}}的{{end}}{{.Guard}}`, `感谢{{.F "data.user_info.uname" "观众"}}开通了{{with .GuardDuration}}{{.}}的{{end}}{{.Guard}}`},
	{"RECALL_DANMU_MSG", i18n.OutputEventRecall, `弹幕已撤回{{with .N "data.target_id"}}（编号{{.}}）{{end}}`, `有一条弹幕被撤回`},
	{"WARNING", i18n.OutputEventWarning, `直播警告：{{.F "msg" "请遵守直播规范"}}`, `收到直播警告。{{.F "msg" "请遵守直播规范"}}`},
	{"ROOM_SILENT_ON", i18n.OutputEventSilentOn, `直播间已开启{{.Silent}}{{with .N "data.level"}}，等级门槛{{.}}{{end}}{{with .N "data.second"}}，时长{{.}}秒{{end}}{{with .F "data.msg" ""}}：{{.}}{{end}}`, `直播间已开启{{.Silent}}{{with .F "data.msg" ""}}。{{.}}{{end}}`},
	{"ROOM_SILENT_OFF", i18n.OutputEventSilentOff, `直播间已关闭禁言规则`, `直播间禁言规则已解除`},
	{"room_admin_entrance", i18n.OutputEventAdminEntrance, `已任命房管{{with .N "uid"}}（用户{{.}}）{{end}}{{with .F "msg" ""}}：{{.}}{{end}}`, `直播间任命了新房管{{with .F "msg" ""}}。{{.}}{{end}}`},
	{"ROOM_ADMIN_REVOKE", i18n.OutputEventAdminRevoke, `已撤销房管{{with .N "uid"}}（用户{{.}}）{{end}}{{with .F "msg" ""}}：{{.}}{{end}}`, `直播间撤销了房管权限{{with .F "msg" ""}}。{{.}}{{end}}`},
}

type templates struct{ overlay, tts *template.Template }

var compiled = func() map[string]templates {
	result := make(map[string]templates, len(specs))
	for _, spec := range specs {
		if _, exists := result[spec.ID]; exists {
			panic("事件模板标识重复")
		}
		result[spec.ID] = templates{
			overlay: template.Must(template.New(spec.ID).Parse(spec.OverlayTemplate)),
			tts:     template.Must(template.New(spec.ID).Parse(spec.TTSTemplate)),
		}
	}
	return result
}()

func Specs() []EventSpec { return append([]EventSpec(nil), specs...) }

func ID(e danmaku.Event) string {
	if e.Kind == "detail" {
		return e.Text
	}
	return e.Kind
}

func Enabled(disabled []string, e danmaku.Event) bool {
	id := ID(e)
	if _, exists := compiled[id]; !exists {
		return false
	}
	for _, value := range disabled {
		if value == id {
			return false
		}
	}
	return true
}

func RenderOverlay(e danmaku.Event) string { return render(e, false) }
func RenderTTS(e danmaku.Event) string     { return render(e, true) }

// OverlayRole identifies the purchase category without inspecting rendered text.
func OverlayRole(e danmaku.Event) byte {
	if e.Kind == "sc" {
		return 's'
	}
	var guard string
	if e.Kind == "guard" {
		guard = e.Gift
	} else if id := ID(e); id == "USER_TOAST_MSG" || id == "LIVE_OPEN_PLATFORM_GUARD" {
		guard = (eventData{e}).Guard()
	}
	switch guard {
	case "舰长":
		return 'c'
	case "提督":
		return 'a'
	case "总督":
		return 'g'
	}
	return 'n'
}

func render(e danmaku.Event, spoken bool) string {
	t, exists := compiled[ID(e)]
	if !exists {
		return ""
	}
	selected, limit := t.overlay, 2048
	if spoken {
		selected, limit = t.tts, 500
	}
	var out strings.Builder
	if err := selected.Execute(&out, eventData{e}); err != nil {
		return ""
	}
	return clean(out.String(), limit)
}

type eventData struct{ danmaku.Event }

func (d eventData) GuardDuration() string {
	count, unit := d.Event.GuardPeriod()
	if count <= 0 {
		return clean(unit, 32)
	}
	if unit == "月" {
		unit = "个月"
	}
	return strconv.FormatInt(count, 10) + unit
}

func (d eventData) User() string { return fallback(clean(d.Event.User, 80), "观众") }
func (d eventData) Text() string {
	if d.Deleted {
		return "内容已删除"
	}
	return fallback(clean(d.Event.Text, 1600), "未提供内容")
}
func (d eventData) Gift() string {
	name := "礼物"
	if d.Kind == "guard" {
		name = "大航海"
	}
	return fallback(clean(d.Event.Gift, 80), name)
}
func (d eventData) Coins() string {
	if d.Amount <= 0 {
		return ""
	}
	unit := ""
	switch d.CoinType {
	case "gold":
		unit = "金瓜子"
	case "silver":
		unit = "银瓜子"
	}
	if unit == "" {
		return ""
	}
	return "（" + strconv.FormatInt(d.Amount, 10) + unit + "）"
}
func (d eventData) field(name string) string {
	name = strings.TrimPrefix(name, "data.")
	for _, field := range d.Fields {
		if field.Name == name {
			return field.Value
		}
	}
	return ""
}
func (d eventData) F(name, missing string) string {
	return fallback(clean(d.field(name), 240), missing)
}
func (d eventData) N(name string) string {
	value := d.field(name)
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 {
		return ""
	}
	return strconv.FormatInt(n, 10)
}
func (d eventData) Guard() string {
	if role := d.F("data.role_name", ""); role != "" {
		return role
	}
	switch d.N("data.guard_level") {
	case "1":
		return "总督"
	case "2":
		return "提督"
	case "3":
		return "舰长"
	}
	return "大航海"
}
func (d eventData) Silent() string {
	switch d.field("data.type") {
	case "level":
		return "等级禁言"
	case "medal":
		return "粉丝勋章禁言"
	case "member":
		return "会员禁言"
	}
	return "禁言规则"
}
func fallback(value, missing string) string {
	if value == "" {
		return missing
	}
	return value
}

// 限制源输入与输出，移除终端转义、控制字符和双向文本控制符。
func clean(value string, limit int) string {
	if len(value) > 16<<10 {
		value = value[:16<<10]
	}
	value = ansi.Strip(value)
	var out strings.Builder
	count := 0
	for _, r := range value {
		if count >= limit {
			break
		}
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) || r == unicode.ReplacementChar {
			if unicode.IsSpace(r) {
				out.WriteByte(' ')
				count++
			}
			continue
		}
		out.WriteRune(r)
		count++
	}
	return strings.TrimSpace(out.String())
}
