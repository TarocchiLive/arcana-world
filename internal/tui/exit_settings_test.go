package tui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"arcana-world/internal/bili"
	"arcana-world/internal/domain"
	"arcana-world/internal/store"
	"github.com/zalando/go-keyring"
)

func TestExitSettingsToggleIndependentlyWithoutStoppingBroadcast(t *testing.T) {
	var requests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request while changing exit settings", http.StatusBadGateway)
	}))
	defer proxy.Close()
	state := &lifecycleOBS{}
	state.active.Store(true)
	endpoint := lifecycleServer(t, state)
	m := lifecycleModel(t, context.Background())
	client, err := bili.New(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	m.client = client
	m.account = &domain.Account{UID: "123", Cookies: map[string]string{"DedeUserID": "123", "bili_jct": "csrf"}}
	m.client.SetAccount(*m.account)
	m.room = &domain.Room{ID: 456, Live: true}
	// Cleanup must not make a real account-room request for this synthetic account.
	t.Cleanup(func() { m.account = nil })
	if err := m.obsClient.Connect(context.Background(), endpoint, ""); err != nil {
		t.Fatal(err)
	}
	m.page = settingsPage
	original := m.store.Config()
	steps := []struct {
		action  string
		obsOff  bool
		liveOff bool
	}{
		{"exit-obs-stop", true, false},
		{"exit-live-stop", true, true},
		{"exit-obs-stop", false, true},
		{"exit-live-stop", false, false},
	}
	for _, step := range steps {
		command := m.perform(step.action)
		if command == nil {
			t.Fatalf("setting action %q did not save", step.action)
		}
		msg := command()
		if result, ok := msg.(resultMsg); !ok || result.err != nil {
			t.Fatalf("setting action failed: %+v", msg)
		}
		m.Update(msg)
		reopened, err := store.OpenWithBackend(m.store.Dir(), lifecycleSecrets{})
		if err != nil {
			t.Fatal(err)
		}
		got := reopened.Config()
		if got.ExitOBSStopDisabled != step.obsOff || got.ExitLiveStopDisabled != step.liveOff {
			t.Fatalf("%s persisted wrong options: OBS off=%v live off=%v", step.action, got.ExitOBSStopDisabled, got.ExitLiveStopDisabled)
		}
		got.ExitOBSStopDisabled, got.ExitLiveStopDisabled = false, false
		if !reflect.DeepEqual(got, original) {
			t.Fatal("exit setting changed unrelated configuration")
		}
		if !state.active.Load() || state.stops.Load() != 0 || requests.Load() != 0 || !m.room.Live {
			t.Fatal("changing exit settings stopped or contacted an active broadcast")
		}
	}
}

func TestExitUsesCommittedToggleBeforeResultDelivery(t *testing.T) {
	state := &lifecycleOBS{}
	state.active.Store(true)
	endpoint := lifecycleServer(t, state)
	m := lifecycleModel(t, context.Background())
	if err := m.obsClient.Connect(context.Background(), endpoint, ""); err != nil {
		t.Fatal(err)
	}
	command := m.perform("exit-obs-stop")
	if command == nil {
		t.Fatal("exit toggle did not produce a save command")
	}
	queued := command().(resultMsg)
	if queued.err != nil {
		t.Fatal(queued.err)
	}
	// Close before Bubble Tea delivers the successful save result.
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if !state.active.Load() || state.stops.Load() != 0 {
		t.Fatal("exit ignored the committed OFF switch while its UI result was queued")
	}
}

func TestQueuedOBSConnectionEditsCannotRunAfterExit(t *testing.T) {
	for _, kind := range []string{"obs-url", "obs-password"} {
		t.Run(kind, func(t *testing.T) {
			m := lifecycleModel(t, context.Background())
			originalURL := m.store.Config().OBSURL
			m.editKind = kind
			m.input.SetValue("ws://127.0.0.1:56789")
			command := m.submitForm()
			if command == nil {
				t.Fatal("valid OBS form did not produce a command")
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			result := command().(resultMsg)
			if !errors.Is(result.err, context.Canceled) {
				t.Fatalf("OBS edit ran after shutdown: %v", result.err)
			}
			if m.store.Config().OBSURL != originalURL {
				t.Fatal("queued edit changed the OBS URL after shutdown")
			}
			if _, err := m.store.OBSSecret(); !errors.Is(err, keyring.ErrNotFound) {
				t.Fatal("queued edit saved an OBS password after shutdown")
			}
		})
	}
}
