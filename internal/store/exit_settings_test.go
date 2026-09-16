package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExitSettingsMigrateAndRoundTripIndependently(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "new-config"
		if legacy {
			name = "legacy-config"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if legacy {
				data := []byte(`{"protocol":"srt","proxy":"http://127.0.0.1:8080","obs_auto_connect":true,"danmaku_disabled":true,"recent_titles":["saved title"]}`)
				if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			backend := memoryBackend{}
			s, err := OpenWithBackend(dir, backend)
			if err != nil {
				t.Fatal(err)
			}
			original := s.Config()
			if original.ExitOBSStopDisabled || original.ExitLiveStopDisabled {
				t.Fatal("new and legacy configurations must enable both exit actions")
			}
			for _, disabled := range [][2]bool{{true, false}, {true, true}, {false, true}, {false, false}} {
				cfg := s.Config()
				cfg.ExitOBSStopDisabled, cfg.ExitLiveStopDisabled = disabled[0], disabled[1]
				if err := s.SaveConfig(cfg); err != nil {
					t.Fatal(err)
				}
				s, err = OpenWithBackend(dir, backend)
				if err != nil {
					t.Fatal(err)
				}
				got := s.Config()
				if got.ExitOBSStopDisabled != disabled[0] || got.ExitLiveStopDisabled != disabled[1] {
					t.Fatalf("exit options did not survive reopening: got OBS=%v live=%v, want %v", got.ExitOBSStopDisabled, got.ExitLiveStopDisabled, disabled)
				}
				got.ExitOBSStopDisabled, got.ExitLiveStopDisabled = false, false
				if !reflect.DeepEqual(got, original) {
					t.Fatal("saving exit options changed unrelated settings")
				}
			}
		})
	}
}
