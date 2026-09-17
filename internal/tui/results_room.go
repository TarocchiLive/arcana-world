package tui

import (
	"arcana-world/internal/app"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
)

func refreshOperation() operation[domain.Room] {
	return operation[domain.Room]{name: "refresh", discardOnCancel: true, handle: (*Model).handleRefreshResult}
}

func (m *Model) handleRefreshResult(value domain.Room, err error) tea.Cmd {

	if cmd, failed := m.resultError("refresh", err); failed {
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
	return operation[app.StartOutcome]{name: "start", discardOnCancel: false, handle: (*Model).handleStartResult}
}

func (m *Model) handleStartResult(value app.StartOutcome, err error) tea.Cmd {

	if cmd, failed := m.resultError("start", err); failed {
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
		m.log(i18n.T(i18n.TUILogLiveOBSFailed) + outcome.OBSErr.Error())
	}
	if s.Warning != "" {
		m.log(s.Warning)
	}
	return m.finishResult()
}

func stopOperation() operation[app.StopOutcome] {
	return operation[app.StopOutcome]{name: "stop", discardOnCancel: false, handle: (*Model).handleStopResult}
}

func (m *Model) handleStopResult(value app.StopOutcome, err error) tea.Cmd {

	if cmd, failed := m.resultError("stop", err); failed {
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
	return operation[areaCatalog]{name: "areas", discardOnCancel: true, handle: (*Model).handleAreasResult}
}

func (m *Model) handleAreasResult(value areaCatalog, err error) tea.Cmd {

	if cmd, failed := m.resultError("areas", err); failed {
		return cmd
	}
	catalog := value
	m.selection = newAreaSelection(catalog.all, catalog.recent, m.room.AreaID)
	m.mode = "selection"
	m.view.GotoTop()
	m.status = i18n.T(i18n.TUIStatusCategorySelectHint)
	if catalog.historyErr != nil {
		m.log(i18n.T(i18n.TUILogRecentCategoriesFailed) + catalog.historyErr.Error())
	}
	return m.selection.Init()
}

func delayOperation() operation[int] {
	return operation[int]{name: "delay", discardOnCancel: true, handle: (*Model).handleDelayResult}
}

func (m *Model) handleDelayResult(value int, err error) tea.Cmd {

	if cmd, failed := m.resultError("delay", err); failed {
		return cmd
	}
	return m.form("delay", i18n.T(i18n.TUIFormDelay), fmt.Sprint(value), false)
}

func areaOperation() operation[*domain.Area] {
	return operation[*domain.Area]{name: "area", discardOnCancel: false, handle: (*Model).handleAreaResult}
}

func (m *Model) handleAreaResult(value *domain.Area, err error) tea.Cmd {
	if value != nil && m.room != nil {
		m.room.AreaID = value.ID
		m.room.AreaName = value.Name
		m.room.ParentName = value.Parent
	}
	if cmd, failed := m.resultError("area", err); failed {
		return cmd
	}
	m.log(fmt.Sprintf(i18n.T(i18n.TUILogOperationSucceeded), operationName("area")))

	return m.finishResult()
}

func titleOperation() operation[*string] {
	return operation[*string]{name: "title", discardOnCancel: false, handle: (*Model).handleTitleResult}
}

func (m *Model) handleTitleResult(value *string, err error) tea.Cmd {
	if value != nil && m.room != nil {
		m.room.Title = *value
	}
	if cmd, failed := m.resultError("title", err); failed {
		return cmd
	}
	m.log(fmt.Sprintf(i18n.T(i18n.TUILogOperationSucceeded), operationName("title")))

	return m.finishResult()
}

func announcementOperation() operation[*string] {
	return operation[*string]{name: "announcement", discardOnCancel: false, handle: (*Model).handleAnnouncementResult}
}

func (m *Model) handleAnnouncementResult(value *string, err error) tea.Cmd {
	if value != nil && m.room != nil {
		m.room.Announcement = *value
	}
	if cmd, failed := m.resultError("announcement", err); failed {
		return cmd
	}
	m.log(fmt.Sprintf(i18n.T(i18n.TUILogOperationSucceeded), operationName("announcement")))

	return m.finishResult()
}

var delaySetOperation = completionOperation("delay-set")
