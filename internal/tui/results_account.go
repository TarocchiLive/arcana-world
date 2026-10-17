package tui

import (
	"arcana-world/internal/app"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"arcana-world/internal/journal"
	tea "charm.land/bubbletea/v2"
	"fmt"
)

func qrOperation() operation[domain.QR] {
	return operation[domain.QR]{label: i18n.TUIOperationGenerateQR, discardOnCancel: true, handle: (*Model).handleQRResult}
}

func (m *Model) handleQRResult(value domain.QR, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	qr := value
	m.qr = &qr
	m.mode = "qr"
	m.qrGeneration++
	m.qrPollCode = -1
	logger.Info("login qr generated", "generation", m.qrGeneration)
	m.view.GotoTop()
	m.progressStatus(i18n.T(i18n.TUIStatusScanQR))
	return tea.Batch(m.prepareQR(qr.URL), m.nextPoll())
}

func pollOperation() operation[domain.LoginPoll] {
	return operation[domain.LoginPoll]{label: i18n.TUIOperationWaitQR, discardOnCancel: true, periodic: true, handle: (*Model).handlePollResult}
}

func (m *Model) handlePollResult(value domain.LoginPoll, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		if cmd != nil {
			return cmd
		}
		m.mode = ""
		m.qr = nil
		m.qrText = ""
		m.qrGeneration++
		return nil
	}
	if m.mode != "qr" {
		return nil
	}
	poll := value
	if poll.Code != m.qrPollCode {
		// 只记录状态变化，不记录每次轮询。
		logger.Info("login poll state changed", "code", poll.Code, "previous_code", m.qrPollCode)
		m.qrPollCode = poll.Code
	}
	switch poll.Code {
	case 0:
		if poll.Account == nil {
			logger.Error("login succeeded without credentials")
			m.warn(i18n.T(i18n.TUILogLoginCredentialsMissing))
			m.mode = ""
			return nil
		}
		m.pendingAccount = poll.Account
		registerAccountSecrets(*poll.Account)
		m.qr = nil
		m.qrText = ""
		m.qrGeneration++
		return m.saveLogin(*poll.Account)
	case 86101:
		m.progressStatus(i18n.T(i18n.TUIStatusWaitingQR))
	case 86090:
		m.progressStatus(i18n.T(i18n.TUIStatusConfirmLogin))
	case 86038:
		logger.Warn("login qr expired", "code", poll.Code)
		m.warn(i18n.T(i18n.TUILogQRExpired))
		m.mode = ""
		m.qr = nil
		m.qrText = ""
		return nil
	default:
		logger.Error("login poll returned unexpected status", "code", poll.Code)
		m.warn(fmt.Sprintf(i18n.T(i18n.TUILogLoginStatusUnexpected), poll.Code))
		m.mode = ""
		return nil
	}
	return m.nextPoll()
}

func accountOperation() operation[app.AccountOutcome] {
	return operation[app.AccountOutcome]{label: i18n.TUIOperationLoadAccount, discardOnCancel: false, handle: (*Model).handleAccountResult}
}

func (m *Model) handleAccountResult(value app.AccountOutcome, err error, label i18n.Key) tea.Cmd {
	m.applyOldRoom(value)
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	return m.adoptAccount(value)
}

func loginSaveOperation() operation[app.AccountOutcome] {
	return operation[app.AccountOutcome]{label: i18n.TUIOperationSaveLogin, discardOnCancel: false, handle: (*Model).handleLoginSaveResult}
}

func (m *Model) handleLoginSaveResult(value app.AccountOutcome, err error, label i18n.Key) tea.Cmd {
	m.applyOldRoom(value)
	if cmd, failed := m.resultError(label, err); failed {
		if cmd != nil {
			return cmd
		}
		if m.canceled {
			m.pendingAccount = nil
		} else {
			m.mode = "login-save"
		}
		return nil
	}
	cmd := m.adoptAccount(value)
	m.logCredentialStorage()
	return cmd
}

func deleteOperation() operation[app.AccountOutcome] {
	return operation[app.AccountOutcome]{label: i18n.TUIOperationRemoveAccount, discardOnCancel: false, handle: (*Model).handleDeleteResult}
}

func (m *Model) handleDeleteResult(value app.AccountOutcome, err error, label i18n.Key) tea.Cmd {
	m.applyOldRoom(value)
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	out := value
	if out.Client != nil {
		m.account = nil
		m.room = nil
		m.stream = nil
		m.reveal = false
		m.session.Release(m.client)
		m.client = out.Client
		m.syncChat()
	}
	m.log(i18n.T(i18n.TUILogAccountRemoved))
	return m.finishResult()
}

func faceOperation() operation[string] {
	return operation[string]{label: i18n.TUIOperationIdentityLink, discardOnCancel: true, handle: (*Model).handleFaceResult}
}

func (m *Model) handleFaceResult(value string, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	logger.Info("identity verification required", "challenge_ready", value != "")
	m.faceURL = value
	m.mode = "face"
	m.view.GotoTop()
	m.warnStatus(i18n.T(i18n.TUIStatusIdentityRequired))
	open := m.prepareQR(m.faceURL)
	return tea.Batch(open, m.finishResult())
}

// 切换或删除账号可能已停止旧直播，但后续本地操作仍可能失败。
func (m *Model) applyOldRoom(value app.AccountOutcome) {
	if value.OldRoom != nil {
		m.room = value.OldRoom
		if !m.room.Live {
			m.stream = nil
			m.reveal = false
		}
		m.obsState = m.obsClient.Snapshot()
	}
}
func (m *Model) adoptAccount(value app.AccountOutcome) tea.Cmd {
	a := value
	m.account = &a.Account
	registerAccountSecrets(a.Account)
	logger.Info("account switched", "uid", a.Account.UID, "previous_room_stopped", a.OldRoom != nil)
	m.session.Release(m.client)
	m.client = a.Client
	m.syncChat()
	m.room = nil
	m.stream = nil
	m.reveal = false
	m.pendingAccount = nil
	m.mode = ""
	m.qr = nil
	m.qrText = ""
	m.qrGeneration++
	m.log(i18n.T(i18n.TUILogAccountSwitched) + a.Account.Name + " / " + a.Account.UID)
	return m.refresh()
}

// registerAccountSecrets 把账号凭据登记到日志脱敏表，防止其意外出现在日志中。
func registerAccountSecrets(a domain.Account) {
	values := make([]string, 0, len(a.Cookies))
	for name, v := range a.Cookies {
		if name != "DedeUserID" && v != "" {
			values = append(values, v)
		}
	}
	journal.AddSecrets(values...)
}
