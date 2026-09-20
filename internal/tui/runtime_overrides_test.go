package tui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"arcana-world/internal/i18n"
	"arcana-world/internal/store"
)

func TestSessionProxySurvivesResetAndUnrelatedSave(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "session-proxy.invalid" {
			t.Errorf("unexpected proxy target: %s", r.URL.Host)
		}
		_, _ = io.WriteString(w, "session proxy")
	}))
	defer proxy.Close()
	proxyURL, disabled := proxy.URL, false
	s, err := store.Open(t.TempDir(), store.Options{CredentialBackend: "memory", Overrides: store.ConfigOverrides{Proxy: &proxyURL, OBSAutoConnect: &disabled, OBSAutoStream: &disabled}})
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	for _, action := range []string{"proxy", "obs-auto-connect", "obs-auto-stream"} {
		if cmd := m.perform(action); cmd != nil || m.status != i18n.T(i18n.TUISessionOverride) {
			t.Fatalf("%s did not reject the overridden setting", action)
		}
	}
	cfg := m.config
	cfg.Protocol = "srt"
	msg := m.saveConfig(cfg, false)().(taskMessage)
	if msg.taskError() != nil {
		t.Fatal(msg.taskError())
	}
	m.Update(msg)
	saved, err := store.ReadConfig(s.Dir())
	if err != nil || saved.Protocol != "srt" || saved.Proxy == proxyURL {
		t.Fatalf("unrelated save persisted override or lost protocol: %+v, %v", saved, err)
	}
	msg = m.resetSettings()().(taskMessage)
	if msg.taskError() != nil {
		t.Fatal(msg.taskError())
	}
	m.Update(msg)
	response, err := m.client.HTTP.Get("http://session-proxy.invalid/check")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "session proxy" {
		t.Fatalf("reset bypassed forced proxy: %q, %v", body, err)
	}
	if m.config.Proxy != proxyURL || m.config.OBSAutoConnect || m.config.OBSAutoStream {
		t.Fatal("reset lost effective session settings")
	}
}

func TestTTSOverrideSurvivesReset(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			m := lifecycleModel(t, context.Background())
			if err := m.ConfigureTTS(enabled); err != nil {
				t.Fatal(err)
			}
			state := m.ttsStateText()
			if cmd := m.performTTS("tts-toggle"); cmd != nil || m.status != i18n.T(i18n.TUISessionOverride) || m.ttsStateText() != state {
				t.Fatal("TTS toggle changed forced state or omitted rejection notice")
			}
			if !enabled {
				if cmd := m.performTTS("tts-preview"); cmd != nil || m.status != i18n.T(i18n.TUISessionOverride) || m.ttsStateText() != state {
					t.Fatal("disabled TTS allowed preview")
				}
			}
			msg := m.resetSettings()().(taskMessage)
			if msg.taskError() != nil {
				t.Fatal(msg.taskError())
			}
			m.Update(msg)
			if m.ttsStateText() != state {
				t.Fatal("settings reset lost forced TTS state")
			}
			if err := m.ConfigureTTS(!enabled); err == nil {
				t.Fatal("second configuration replaced session override")
			}
		})
	}
}
