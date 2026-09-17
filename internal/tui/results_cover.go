package tui

import (
	"arcana-world/internal/bili"
	"arcana-world/internal/coverimage"
	"arcana-world/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
	"image"
)

func coverPrepareOperation() operation[*coverimage.Prepared] {
	return operation[*coverimage.Prepared]{label: i18n.TUIOperationPrepareCover, discardOnCancel: true, handle: (*Model).handleCoverPrepareResult}
}

func (m *Model) handleCoverPrepareResult(value *coverimage.Prepared, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		if cmd != nil {
			return cmd
		}
		m.mode = "cover"
		return nil
	}
	m.cover = value
	m.mode = "cover-review"
	m.view.GotoTop()
	m.status = i18n.T(i18n.TUIStatusCoverProcessed)
	return m.finishResult()
}

func coverFetchOperation() operation[image.Image] {
	return operation[image.Image]{label: i18n.TUIOperationFetchCover, discardOnCancel: true, handle: (*Model).handleCoverFetchResult}
}

func (m *Model) handleCoverFetchResult(value image.Image, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		if cmd != nil {
			return cmd
		}
		m.mode = "cover"
		return nil
	}
	return m.previewCover(value, i18n.T(i18n.TUICoverCurrentPreviewTitle))
}

func coverUploadOperation() operation[bili.CoverUpdate] {
	return operation[bili.CoverUpdate]{label: i18n.TUIOperationUploadCover, discardOnCancel: false, handle: (*Model).handleCoverUploadResult}
}

func (m *Model) handleCoverUploadResult(value bili.CoverUpdate, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	update := value
	m.room.CoverURL = update.URL
	m.room.CoverStatus = update.Status
	if update.Reason != "" {
		m.room.CoverStatus += " · " + m.safe(update.Reason)
	}
	m.cover = nil
	m.mode = "cover"
	m.log(i18n.T(i18n.TUILogCoverSubmitted) + m.room.CoverStatus)
	return m.finishResult()
}
