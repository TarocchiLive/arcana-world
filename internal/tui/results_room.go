package tui

import (
	"arcana-world/internal/app"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
	"fmt"
)

func refreshOperation() operation[domain.Room] {
	return operation[domain.Room]{label: i18n.TUIOperationRefreshRoom, discardOnCancel: true, handle: (*Model).handleRefreshResult}
}

func (m *Model) handleRefreshResult(value domain.Room, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	room := value
	m.room = &room
	if !room.Live {
		m.stream = nil
		m.reveal = false
	}
	m.log(i18n.T(i18n.TUILogRoomRefreshed))
	return m.finishResult()
}

func startOperation() operation[app.StartOutcome] {
	return operation[app.StartOutcome]{label: i18n.TUIOperationStartLive, discardOnCancel: false, handle: (*Model).handleStartResult}
}

func (m *Model) handleStartResult(value app.StartOutcome, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	outcome := value
	s := outcome.Stream
	m.stream = &s
	m.reveal = false
	if m.room != nil {
		m.room.Live = true
	}
	m.log(i18n.T(i18n.TUILogLiveStarted))
	if outcome.OBSStarted {
		m.log(i18n.T(i18n.TUILogLiveOBSStarted))
	} else if outcome.OBSConfigured {
		m.log(i18n.T(i18n.TUILogLiveOBSConfigured))
	}
	if outcome.OBSErr != nil {
		m.warn(i18n.T(i18n.TUILogLiveOBSFailed) + outcome.OBSErr.Error())
	}
	if s.Warning != "" {
		m.warn(s.Warning)
	}
	return m.finishResult()
}

func stopOperation() operation[app.StopOutcome] {
	return operation[app.StopOutcome]{label: i18n.TUIOperationStopLive, discardOnCancel: false, handle: (*Model).handleStopResult}
}

func (m *Model) handleStopResult(value app.StopOutcome, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	if m.room != nil {
		m.room.Live = false
	}
	m.stream = nil
	m.reveal = false
	m.log(i18n.T(i18n.TUILogLiveStopped))
	if value.OBSStopped {
		m.log(i18n.T(i18n.TUILogLiveOBSStopped))
	}
	return m.finishResult()
}

func areasOperation() operation[areaCatalog] {
	return operation[areaCatalog]{label: i18n.TUIOperationFetchCategories, discardOnCancel: true, handle: (*Model).handleAreasResult}
}

func (m *Model) handleAreasResult(value areaCatalog, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	catalog := value
	m.selection = newAreaSelection(catalog.all, catalog.recent, m.room.AreaID)
	m.mode = "selection"
	m.view.GotoTop()
	m.progressStatus(i18n.T(i18n.TUIStatusCategorySelectHint))
	if catalog.historyErr != nil {
		m.warn(i18n.T(i18n.TUILogRecentCategoriesFailed) + catalog.historyErr.Error())
	}
	return m.selection.Init()
}

func delayOperation() operation[int] {
	return operation[int]{label: i18n.TUIOperationReadDelay, discardOnCancel: true, handle: (*Model).handleDelayResult}
}

func (m *Model) handleDelayResult(value int, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	return m.form("delay", i18n.T(i18n.TUIFormDelay), fmt.Sprint(value), false)
}

func areaOperation() operation[*domain.Area] {
	return operation[*domain.Area]{label: i18n.TUIOperationUpdateCategory, discardOnCancel: false, handle: (*Model).handleAreaResult}
}

func (m *Model) handleAreaResult(value *domain.Area, err error, label i18n.Key) tea.Cmd {
	if value != nil && m.room != nil {
		m.room.AreaID = value.ID
		m.room.AreaName = value.Name
		m.room.ParentName = value.Parent
	}
	return m.completeOperation(label, err)
}

func titleOperation() operation[*string] {
	return operation[*string]{label: i18n.TUIOperationUpdateTitle, discardOnCancel: false, handle: (*Model).handleTitleResult}
}

func (m *Model) handleTitleResult(value *string, err error, label i18n.Key) tea.Cmd {
	if value != nil && m.room != nil {
		m.room.Title = *value
	}
	return m.completeOperation(label, err)
}

func announcementOperation() operation[*string] {
	return operation[*string]{label: i18n.TUIOperationUpdateAnnouncement, discardOnCancel: false, handle: (*Model).handleAnnouncementResult}
}

func (m *Model) handleAnnouncementResult(value *string, err error, label i18n.Key) tea.Cmd {
	if value != nil && m.room != nil {
		m.room.Announcement = *value
	}
	return m.completeOperation(label, err)
}

var delaySetOperation = completionOperation(i18n.TUIOperationUpdateDelay)
