package tui

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"arcana-world/internal/domain"
	"arcana-world/internal/store"
	tea "charm.land/bubbletea/v2"
)

func coverTestModel(t *testing.T) (*Model, string) {
	t.Helper()
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Config()
	cfg.ExitLiveStopDisabled = true
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	a := domain.Account{UID: "1", Cookies: map[string]string{"SESSDATA": "test-session", "bili_jct": "test-csrf"}}
	m.account = &a
	m.client.SetAccount(a)
	m.room = &domain.Room{ID: 77, CoverURL: "https://i0.hdslb.com/old.png"}
	var data bytes.Buffer
	if err = png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 64, 64))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "input.png")
	if err = os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return m, path
}
func TestCoverRequiresConfirmationBeforePublishing(t *testing.T) {
	m, path := coverTestModel(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/x/upload/web/image":
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				t.Error(err)
				return
			}
			defer r.MultipartForm.RemoveAll()
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Error(err)
				return
			}
			defer file.Close()
			img, err := png.Decode(file)
			if err != nil || img.Bounds() != image.Rect(0, 0, 704, 396) {
				t.Error("unprocessed image uploaded")
				return
			}
			w.Write([]byte(`{"code":0,"data":{"location":"https://i0.hdslb.com/new.png"}}`))
		default:
			w.Write([]byte(`{"code":0,"data":{"audit_info":{"audit_title_status":0}}}`))
		}
	}))
	defer server.Close()
	m.client.APIBase = server.URL
	m.client.LiveBase = server.URL
	m.perform("cover")
	m.coverKey(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m.input.SetValue(path)
	prepare := m.submitForm()
	m.Update(prepare())
	if requests.Load() != 0 {
		t.Fatal("preparing cover mutated remote room")
	}
	cancel := tea.KeyPressMsg{Code: tea.KeyEsc}
	m.Update(cancel)
	if requests.Load() != 0 || m.room.CoverURL != "https://i0.hdslb.com/old.png" {
		t.Fatal("canceling candidate changed published cover")
	}
	prepare = m.prepareCover(path)
	m.Update(prepare())
	_, upload := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if upload == nil {
		t.Fatal("confirmation did not upload")
	}
	m.Update(upload())
	if requests.Load() != 2 || m.room.CoverURL != "https://i0.hdslb.com/new.png" {
		t.Fatal("confirmed cover was not published and refreshed")
	}
}
func TestCanceledCoverPreparationDoesNotReopenReview(t *testing.T) {
	m, path := coverTestModel(t)
	prepare := m.prepareCover(path)
	queued := prepare()
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	_, next := m.Update(queued)
	if next != nil || m.cover != nil || m.mode != "" {
		t.Fatal("canceled preparation reopened a stale review")
	}
}
