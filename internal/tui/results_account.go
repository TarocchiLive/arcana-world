package tui

import (
	"arcana-world/internal/app"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
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
	m.qrText = renderQR(qr.URL)
	m.view.GotoTop()
	m.status = i18n.T(i18n.TUIStatusScanQR)
	return m.nextPoll()
}

func pollOperation() operation[domain.LoginPoll] {
	return operation[domain.LoginPoll]{label: i18n.TUIOperationWaitQR, discardOnCancel: true, handle: (*Model).handlePollResult}
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
	switch poll.Code {
	case 0:
		if poll.Account == nil {
			m.log(i18n.T(i18n.TUILogLoginCredentialsMissing))
			m.mode = ""
			return nil
		}
		m.pendingAccount = poll.Account
		m.qr = nil
		m.qrText = ""
		m.qrGeneration++
		return m.saveLogin(*poll.Account)
	case 86101:
		m.status = i18n.T(i18n.TUIStatusWaitingQR)
	case 86090:
		m.status = i18n.T(i18n.TUIStatusConfirmLogin)
	case 86038:
		m.log(i18n.T(i18n.TUILogQRExpired))
		m.mode = ""
		m.qr = nil
		m.qrText = ""
		return nil
	default:
		m.log(fmt.Sprintf(i18n.T(i18n.TUILogLoginStatusUnexpected), poll.Code))
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
	return m.adoptAccount(value)
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
	m.faceURL = value
	m.qrText = renderQR(m.faceURL)
	m.mode = "face"
	m.view.GotoTop()
	m.status = i18n.T(i18n.TUIStatusIdentityRequired)
	return m.finishResult()
}

// A switch/delete can stop the old broadcast before a later local step fails.
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
