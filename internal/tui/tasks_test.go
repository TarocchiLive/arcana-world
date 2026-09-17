package tui

import (
	"context"
	"errors"
	"testing"

	"arcana-world/internal/domain"
)

func TestConfirmedEditSurvivesCancellationAndLocalFailure(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.room = &domain.Room{ID: 1, Title: "old"}
	m.operation = 7
	m.busy = true
	m.canceled = true
	title := "confirmed remotely"
	m.Update(taskResult[*string]{id: 7, operation: titleOperation(), value: &title, err: errors.New("history save failed")})
	if m.room.Title != title || m.busy {
		t.Fatal("cancellation or local failure discarded the confirmed remote title")
	}
}

func TestStaleTaskCannotClearBusyOrOpenQR(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.operation = 8
	m.busy = true
	_, cmd := m.Update(taskResult[domain.QR]{id: 7, operation: qrOperation(), value: domain.QR{Key: "stale", URL: "https://example.com/stale"}})
	if !m.busy || m.qr != nil || m.mode == "qr" || cmd != nil {
		t.Fatal("stale operation changed the current operation or reopened QR login")
	}
}
