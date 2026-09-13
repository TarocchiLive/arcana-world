package tui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/skip2/go-qrcode"
)

var (
	accent        = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	muted         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("232")).Background(lipgloss.Color("81")).Bold(true)
	warning       = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
)

func (m *Model) View() string {
	width := max(12, m.width-4)
	account := "未登录"
	if m.account != nil {
		account = clean(m.account.Name) + " / " + m.account.UID
	}
	header := accent.Render("ARCANA WORLD") + "  " + muted.Render("BILIBILI LIVE CONTROL")
	var tabs []string
	for i, p := range pages {
		label := fmt.Sprintf(" %d %s ", i+1, p)
		if i == m.page {
			label = selectedStyle.Render(label)
		} else {
			label = muted.Render(label)
		}
		tabs = append(tabs, label)
	}
	m.view.SetContent(m.content())
	status := m.status
	if m.busy || m.obsBusy {
		status = "[进行中] " + status
	}
	footer := "↑↓ / j k 选择 · Enter 执行 · Tab / 1–7 切页 · PgUp/PgDn 滚动 · r 刷新 · q 退出"
	if m.mode != "" {
		footer = "Esc 返回 / 取消 · Enter 确认 · PgUp/PgDn 滚动 · Ctrl+C 退出"
	}
	if m.busy {
		footer = "Esc 取消当前操作 · Ctrl+C 退出（退出不会自动关播）"
	}
	if m.obsBusy && !m.busy {
		footer = "OBS 操作中，请勿重复连接 · Esc 取消 · 可切换其他页面"
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(
		lipgloss.NewStyle().MaxWidth(width).Render(header) + "\n" +
			muted.Render("账号  "+account) + "\n" + strings.Join(tabs, "") + "\n\n" +
			m.view.View() + "\n" +
			lipgloss.NewStyle().MaxWidth(width).Render(warning.Render(clean(status))) + "\n" +
			lipgloss.NewStyle().MaxWidth(width).Render(muted.Render(footer)))
}
func (m *Model) content() string {
	switch m.mode {
	case "cover", "cover-review":
		return m.coverView()
	case "selection":
		return m.selection.View(m.view.Width, m.view.Height)
	case "form":
		if m.editKind == "cover-path" {
			return accent.Render(m.prompt) + "\n\n" + m.input.View() + "\n\n704×396 · 16:9 · 非等比例图片居中裁剪\nEnter 处理并确认 · Esc 返回"
		}
		return accent.Render(m.prompt) + "\n\n" + m.input.View() + "\n\n" + muted.Render("Enter 提交 · Esc 放弃；网络操作在后台执行")
	case "confirm":
		no, yes := " 取消 ", " 确认执行 "
		if m.selected == 0 {
			no = selectedStyle.Render(no)
		} else {
			yes = selectedStyle.Render(yes)
		}
		return warning.Render(clean(m.prompt)) + "\n\n" + no + "    " + yes + "\n\n" + muted.Render("← → / Tab 切换 · Enter 确认")
	case "pick":
		var b strings.Builder
		b.WriteString(accent.Render(m.prompt) + "\n\n")
		window := max(3, m.view.Height-5)
		start := max(0, m.selected-window+1)
		end := min(len(m.choices), start+window)
		for i := start; i < end; i++ {
			label := clean(m.choices[i].label)
			if i == m.selected {
				b.WriteString(selectedStyle.Render(" › "+label) + "\n")
			} else {
				b.WriteString("   " + label + "\n")
			}
		}
		fmt.Fprintf(&b, "\n%d / %d · ↑↓ 选择 · PgUp/PgDn 翻页 · Home/End 首尾", m.selected+1, len(m.choices))
		return b.String()
	case "qr", "face":
		title := "使用哔哩哔哩 App 扫码登录"
		link := ""
		if m.qr != nil {
			link = m.qr.URL
		}
		if m.mode == "face" {
			title = "完成 B 站身份验证后，返回并重新开播"
			link = m.faceURL
		}
		return accent.Render(title) + "\n" + muted.Render("二维码需完整显示；终端过小时请放大窗口，或在浏览器中打开下方链接。") + "\n\n" + m.qrText + "\n" + clean(link) + "\n\n" + muted.Render("链接含临时令牌，请勿分享。Esc 返回。")
	case "login-save":
		return warning.Render("登录凭据尚未保存") + "\n\n请解锁系统密钥环 / Secret Service 后按 Enter 重试。\nEsc 放弃本次登录；不会把 cookies 明文写入磁盘。"
	}
	var b strings.Builder
	switch m.page {
	case 0:
		b.WriteString(accent.Render("直播控制台") + "\n\n")
		if m.room == nil {
			b.WriteString("暂无直播间信息。请先在账号页登录，再刷新。\n")
		} else {
			state := "未开播"
			if m.room.Live {
				state = "直播中"
			}
			fmt.Fprintf(&b, "状态  %s     房间  %d\n标题  %s\n分区  %s / %s (%d)\n", state, m.room.ID, clean(m.room.Title), clean(m.room.ParentName), clean(m.room.AreaName), m.room.AreaID)
		}
		if m.stream != nil {
			fmt.Fprintf(&b, "\n协议  %s\n", m.stream.Protocol)
			if m.reveal {
				fmt.Fprintf(&b, "推流地址  %s\n推流密钥  %s\n", clean(m.stream.Address), clean(m.stream.Key))
			} else {
				b.WriteString("推流地址 / 密钥已隐藏；写入 OBS 不需要显示密钥。\n")
			}
		}
		if m.config.OBSAutoStream {
			b.WriteString("\n自动推流已开启：开播会同步启动 OBS，停播会同步停止 OBS。\n")
		} else {
			b.WriteString("\n连接 OBS 后开播自动写入推流配置；开启自动推流即可同步开停播。\n")
		}
	case 1:
		b.WriteString(accent.Render("账号管理") + "\n\n扫码登录 · 多账号切换 · 系统密钥环存储\n切换失败时保留当前账号；网络错误不会删除凭据。\n")
	case 2:
		b.WriteString(accent.Render("直播间设置") + "\n\n")
		if m.room != nil {
			fmt.Fprintf(&b, "标题  %s\n公告  %s\n封面  %s\n审核  %s\n", clean(m.room.Title), clean(m.room.Announcement), clean(m.room.CoverURL), clean(m.room.CoverStatus))
		}
		b.WriteString("\n封面输出 704×396（16:9），可预览后确认上传。\n")
	case 3:
		b.WriteString(accent.Render("OBS STUDIO · WEBSOCKET V5") + "\n\n")
		fmt.Fprintf(&b, "地址  %s\n", clean(m.config.OBSURL))
		state := "未连接"
		if m.obsState.Connecting || m.obsBusy {
			state = "操作中…"
		} else if m.obsState.Connected {
			state = "已连接（持续会话）"
		}
		fmt.Fprintf(&b, "连接  %s\n", state)
		if m.obsState.Connected {
			streaming := "未推流"
			if m.obsState.Status.Active {
				streaming = "正在推流"
			}
			if m.obsState.Status.Reconnecting {
				streaming = "OBS 推流重连中"
			}
			fmt.Fprintf(&b, "推流  %s\n", streaming)
		}
		b.WriteString("\n自动连接：启动或开播时连接 OBS。\n自动推流：同步开停播。\n连接配置在「设置」页；断开连接不停止推流。\n")
	case 4:
		proxy := m.config.Proxy
		if proxy == "" {
			proxy = "系统环境变量"
		} else if u, e := url.Parse(proxy); e == nil && u.User != nil {
			u.User = url.User("***")
			proxy = u.String()
		}
		fmt.Fprintf(&b, "%s\n\n代理      %s\n推流协议  %s\nOBS 地址  %s\n\n所有 HTTPS 请求均验证证书。\n账号 cookies / OBS 密码不保存在配置文件中。\n修改 OBS 地址或密码会断开旧会话，请重新连接。\n", accent.Render("应用设置"), clean(proxy), m.config.Protocol, clean(m.config.OBSURL))
	case 5:
		b.WriteString(accent.Render("会话日志 · 最近 200 条") + "\n" + muted.Render(clean(m.journal.Path())) + "\n\n")
		if len(m.logs) == 0 {
			b.WriteString("暂无操作记录。")
		}
		for _, line := range m.logs {
			b.WriteString(line + "\n")
		}
	case 6:
		b.WriteString(lipgloss.NewStyle().Width(m.view.Width).Render(helpText))
	}
	items := m.menu()
	if len(items) > 0 {
		b.WriteString("\n")
		cursor := min(m.cursors[m.page], len(items)-1)
		for i, item := range items {
			label := "   " + item.label
			if i == cursor {
				label = selectedStyle.Render(" › " + item.label)
			}
			b.WriteString(label + "\n")
		}
	}
	return b.String()
}
func renderQR(link string) string {
	qr, err := qrcode.New(link, qrcode.Low)
	if err != nil {
		return "链接过长，无法生成终端二维码，请使用链接。\n"
	}
	bitmap := qr.Bitmap()
	var b strings.Builder
	// Explicit black/white colors preserve contrast on both light and dark terminals.
	for y := 0; y < len(bitmap); y += 2 {
		b.WriteString("\x1b[30;47m")
		for x, top := range bitmap[y] {
			bottom := false
			if y+1 < len(bitmap) {
				bottom = bitmap[y+1][x]
			}
			switch {
			case top && bottom:
				b.WriteRune('█')
			case top:
				b.WriteRune('▀')
			case bottom:
				b.WriteRune('▄')
			default:
				b.WriteByte(' ')
			}
		}
		b.WriteString("\x1b[0m\n")
	}
	return b.String()
}

const helpText = `开播指引

1. 使用bilibili手机版扫码登录。
2. 直播间页：设置标题、分区和封面。
3. 请在OBS Studio中启用插件 websocket，并配置好ws服务与密码，并在arcana-world的设置页设置好相应的地址和密码，例如 ws://127.0.0.1:4455
4. OBS 页：连接，开启「自动推流」。
5. 直播页：选择「开播」；如需人脸验证，完成后重试。

未开启自动推流时，请在 OBS 中手动开始推流。
结束直播时，在直播页选择「停播」；未开启自动推流时，还需手动停止 OBS。
`
