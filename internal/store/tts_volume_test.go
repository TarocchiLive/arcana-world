package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTTSVolumeMigrationAndMute(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"tts":{"voice":"zh-CN-XiaoxiaoNeural"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenWithBackend(dir, memoryBackend{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Config()
	if cfg.TTS.Volume != 80 {
		t.Fatalf("legacy profile volume = %d, want 80", cfg.TTS.Volume)
	}
	cfg.TTS.Volume = 0
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithBackend(dir, memoryBackend{})
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Config().TTS.Volume; got != 0 {
		t.Fatalf("saved mute became volume %d after reopening", got)
	}
	cfg.TTS.Volume = 101
	if err := reopened.SaveConfig(cfg); err == nil {
		t.Fatal("out-of-range volume was saved")
	}
	if got := reopened.Config().TTS.Volume; got != 0 {
		t.Fatalf("rejected save changed volume to %d", got)
	}
}
