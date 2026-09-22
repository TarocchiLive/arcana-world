package tui

import (
	"context"
	"fmt"
	"image"
	"strings"

	"arcana-world/internal/bili"
	"arcana-world/internal/coverimage"
	"arcana-world/internal/i18n"
	"arcana-world/internal/termimage"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type coverPreviewFinished struct{ err error }

func (m *Model) coverKey(msg tea.KeyPressMsg) tea.Cmd {
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
		return m.form("cover-path", i18n.T(i18n.TUIFormCoverPath), "", false)
	case "p":
		if m.mode == "cover-review" && m.cover != nil {
			return m.previewCover(m.cover.Image, i18n.T(i18n.TUICoverUploadPreviewTitle))
		}
		if m.room == nil || m.room.CoverURL == "" {
			m.warnStatus(i18n.T(i18n.TUIStatusCoverMissing))
			return nil
		}
		source := m.room.CoverURL
		return work(m, coverFetchOperation(), func(ctx context.Context) (image.Image, error) {
			return m.client.FetchCover(ctx, source)
		})
	case "enter":
		if m.mode == "cover-review" && m.cover != nil {
			prepared := m.cover
			return work(m, coverUploadOperation(), func(ctx context.Context) (bili.CoverUpdate, error) { return m.client.UploadCover(ctx, prepared) })
		}
	}
	return nil
}
func (m *Model) prepareCover(path string) tea.Cmd {
	return work(m, coverPrepareOperation(), func(ctx context.Context) (*coverimage.Prepared, error) { return coverimage.Prepare(ctx, path) })
}
func (m *Model) previewCover(img image.Image, caption string) tea.Cmd {
	if m.previewing {
		return nil
	}
	m.previewing = true
	// Exec 暂停 Bubble Tea 的渲染和输入读取；预览器退出前独占终端 I/O，
	// 避免图像数据块与界面重绘发生竞争。
	return tea.Exec(termimage.New(m.ctx, img, caption), func(err error) tea.Msg { return coverPreviewFinished{err: err} })
}
func (m *Model) coverView() string {
	width := max(1, m.view.Width())
	if m.mode == "cover-review" && m.cover != nil {
		processing := i18n.T(i18n.TUICoverScaled)
		if m.cover.Cropped {
			processing = i18n.T(i18n.TUICoverCropped)
		}
		details := fmt.Sprintf(i18n.T(i18n.TUICoverProcessedDetails), m.cover.SourceWidth, m.cover.SourceHeight, coverimage.Width, coverimage.Height, float64(len(m.cover.PNG))/1024, processing)
		return lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().PaddingBottom(1).Render(m.theme.sectionTitle(i18n.T(i18n.TUICoverConfirmTitle), width)),
			m.theme.textStyle.Width(width).PaddingBottom(1).Render(clean(strings.TrimSpace(details))),
			m.theme.hintText(i18n.T(i18n.TUICoverReviewControls), width))
	}
	state := i18n.T(i18n.TUICoverEmpty)
	if m.room != nil && m.room.CoverURL != "" {
		state = m.room.CoverStatus
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().PaddingBottom(1).Render(m.theme.sectionTitle(i18n.T(i18n.TUICoverTitle), width)),
		m.theme.textStyle.Width(width).PaddingBottom(1).Render(clean(strings.TrimLeft(i18n.T(i18n.TUICoverRequirements), "\n")+state)),
		m.theme.hintText(i18n.T(i18n.TUICoverControls), width))
}
