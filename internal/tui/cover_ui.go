package tui

import (
	"context"
	"fmt"
	"image"

	"arcana-world/internal/coverimage"
	"arcana-world/internal/termimage"
	tea "github.com/charmbracelet/bubbletea"
)

type coverPreviewFinished struct{ err error }

func (m *Model) coverKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		if m.mode == "cover-review" {
			m.mode = "cover"
		} else {
			m.mode = ""
		}
		m.cover = nil
		m.view.GotoTop()
	case "u":
		m.cover = nil
		return m.form("cover-path", "封面图片路径（PNG / JPG / GIF）", "", false)
	case "p":
		if m.mode == "cover-review" && m.cover != nil {
			return m.previewCover(m.cover.Image, "待上传封面 · 704×396 · 16:9")
		}
		if m.room == nil || m.room.CoverURL == "" {
			m.status = "暂无封面，请按 u 选择图片"
			return nil
		}
		source := m.room.CoverURL
		return m.work("cover-fetch", func(ctx context.Context) (any, error) {
			return m.client.FetchCover(ctx, source)
		})
	case "enter":
		if m.mode == "cover-review" && m.cover != nil {
			prepared := m.cover
			return m.work("cover-upload", func(ctx context.Context) (any, error) { return m.client.UploadCover(ctx, prepared) })
		}
	}
	return nil
}
func (m *Model) prepareCover(path string) tea.Cmd {
	return m.work("cover-prepare", func(ctx context.Context) (any, error) { return coverimage.Prepare(ctx, path) })
}
func (m *Model) previewCover(img image.Image, caption string) tea.Cmd {
	if m.previewing {
		return nil
	}
	m.previewing = true
	// Exec pauses Bubble Tea's renderer and input reader. The image viewer owns
	// terminal I/O until it exits, so graphics chunks cannot race TUI redraws.
	return tea.Exec(termimage.New(m.ctx, img, caption), func(err error) tea.Msg { return coverPreviewFinished{err: err} })
}
func (m *Model) coverView() string {
	if m.mode == "cover-review" && m.cover != nil {
		processing := "等比缩放"
		if m.cover.Cropped {
			processing = "居中裁剪为 16:9 后缩放"
		}
		return accent.Render("确认封面") + fmt.Sprintf("\n\n原图  %d×%d\n输出  %d×%d · PNG · %.0f KiB\n处理  %s\n\n", m.cover.SourceWidth, m.cover.SourceHeight, coverimage.Width, coverimage.Height, float64(len(m.cover.PNG))/1024, processing) +
			"p 预览处理结果 · Enter 确认上传\nu 更换图片 · Esc 返回"
	}
	state := "暂无封面"
	if m.room != nil && m.room.CoverURL != "" {
		state = m.room.CoverStatus
	}
	return accent.Render("直播封面") + "\n\n输出  704×396 · 16:9\n支持  PNG / JPG / GIF（首帧），文件不超过 20 MiB\n处理  非 16:9 图片居中裁剪，上传前可预览\n审核  " + clean(state) + "\n\n" +
		"p 预览当前封面 · u 选择上传图片 · Esc 返回"
}
