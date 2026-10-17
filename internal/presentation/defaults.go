package presentation

// 默认模板按稳定事件标识关联，不依赖目录排列顺序。
var englishTemplates = map[string]struct{ overlay, tts string }{
	"chat":                         {`{{.User}}: {{.Text}}`, `{{.User}} says: {{.Text}}`},
	"gift":                         {`{{.User}} sent {{.Gift}}{{if gt .Count 0}} × {{.Count}}{{end}}{{.Coins}}`, `Thank you {{.User}} for {{if gt .Count 0}}{{.Count}} {{end}}{{.Gift}}`},
	"sc":                           {`Super Chat from {{.User}}{{if gt .Amount 0}} ({{.Amount}} yuan){{end}}: {{.Text}}`, `{{.User}} sent a {{if gt .Amount 0}}{{.Amount}} yuan {{end}}Super Chat: {{.Text}}`},
	"guard":                        {`{{.User}} purchased {{.Gift}}{{with .GuardDuration}} for {{.}}{{end}}`, `Thank you {{.User}} for purchasing {{.Gift}}{{with .GuardDuration}} for {{.}}{{end}}`},
	"enter":                        {`Welcome {{.User}} to the room`, `Welcome {{.User}} to the stream`},
	"follow":                       {`{{.User}} followed the streamer`, `Thank you {{.User}} for following`},
	"like":                         {`{{.User}} liked the stream`, `Thank you {{.User}} for the like`},
	"live":                         {`The stream has started`, `The streamer is live`},
	"preparing":                    {`The stream has ended`, `The stream has ended`},
	"room_change":                  {`Room title updated: {{.Text}}`, `The new room title is: {{.Text}}`},
	"room_block":                   {`{{.User}} was muted`, `{{.User}} was muted in the room`},
	"cut_off":                      {`The stream was cut off: {{.Text}}`, `The stream was cut off. {{.Text}}`},
	"delete":                       {`Super Chat deleted`, `A Super Chat was deleted`},
	"USER_TOAST_MSG":               {`{{.F "data.username" "Viewer"}} purchased {{.Guard}}{{with .GuardDuration}} for {{.}}{{end}}`, `Thank you {{.F "data.username" "Viewer"}} for purchasing {{.Guard}}{{with .GuardDuration}} for {{.}}{{end}}`},
	"GUIARD_MSG":                   {`Guard notice: {{.F "msg" "Someone purchased Guard membership"}}`, `Guard notice. {{.F "msg" "Someone purchased Guard membership"}}`},
	"LIVE_OPEN_PLATFORM_SEND_GIFT": {`{{.F "data.uname" "Viewer"}} sent {{.F "data.gift_name" "gift"}}{{with .N "data.gift_num"}} × {{.}}{{end}}`, `Thank you {{.F "data.uname" "Viewer"}} for {{with .N "data.gift_num"}}{{.}} {{end}}{{.F "data.gift_name" "gift"}}`},
	"LIVE_OPEN_PLATFORM_GUARD":     {`{{.F "data.user_info.uname" "Viewer"}} purchased {{.Guard}}{{with .GuardDuration}} for {{.}}{{end}}`, `Thank you {{.F "data.user_info.uname" "Viewer"}} for purchasing {{.Guard}}{{with .GuardDuration}} for {{.}}{{end}}`},
	"RECALL_DANMU_MSG":             {`Chat message recalled{{with .N "data.target_id"}} (ID {{.}}){{end}}`, `A chat message was recalled`},
	"WARNING":                      {`Stream warning: {{.F "msg" "Please follow the streaming rules"}}`, `Stream warning. {{.F "msg" "Please follow the streaming rules"}}`},
	"ROOM_SILENT_ON":               {`The room enabled {{.Silent}}{{with .N "data.level"}}, minimum level {{.}}{{end}}{{with .N "data.second"}}, duration {{.}} seconds{{end}}{{with .F "data.msg" ""}}: {{.}}{{end}}`, `The room enabled {{.Silent}}{{with .F "data.msg" ""}}. {{.}}{{end}}`},
	"ROOM_SILENT_OFF":              {`Room chat restrictions disabled`, `Room chat restrictions have been lifted`},
	"room_admin_entrance":          {`Moderator appointed{{with .N "uid"}} (user {{.}}){{end}}{{with .F "msg" ""}}: {{.}}{{end}}`, `A new moderator was appointed{{with .F "msg" ""}}. {{.}}{{end}}`},
	"ROOM_ADMIN_REVOKE":            {`Moderator revoked{{with .N "uid"}} (user {{.}}){{end}}{{with .F "msg" ""}}: {{.}}{{end}}`, `Moderator privileges were revoked{{with .F "msg" ""}}. {{.}}{{end}}`},
}

func localized(english bool, zh, en string) string {
	if english {
		return en
	}
	return zh
}

var eventIDs = func() map[string]struct{} {
	ids := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		if _, exists := ids[spec.ID]; exists {
			panic("事件模板标识重复")
		}
		ids[spec.ID] = struct{}{}
	}
	return ids
}()

func knownEvent(id string) bool {
	_, ok := eventIDs[id]
	return ok
}

func eventFieldHint(id string) string {
	switch id {
	case "chat":
		return ".User .Text .Time"
	case "gift":
		return ".User .Gift .Count .Amount .CoinType .Coins .Time"
	case "sc":
		return ".User .Text .Amount .Time"
	case "guard", "USER_TOAST_MSG", "LIVE_OPEN_PLATFORM_GUARD":
		return ".Guard .Gift .GuardDuration .Time"
	case "enter", "follow", "like", "room_block":
		return ".User .Time"
	case "room_change", "cut_off":
		return ".Text .Time"
	case "ROOM_SILENT_ON":
		return ".Silent .Time"
	default:
		return ".Time"
	}
}
