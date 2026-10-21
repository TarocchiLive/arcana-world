package tts

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type synthFunc func(context.Context, string) ([]byte, error)

func (f synthFunc) Synthesize(c context.Context, s string) ([]byte, error) { return f(c, s) }
func (f synthFunc) Close() error                                           { return nil }

type playFunc func(context.Context, []byte) error

func (f playFunc) Play(c context.Context, b []byte) error { return f(c, b) }
func (f playFunc) Close() error                           { return nil }
func receive[T any](t *testing.T, c <-chan T) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
		var zero T
		return zero
	}
}
func TestStopAndReuse(t *testing.T) {
	for _, stage := range []string{"synthesis", "playback"} {
		t.Run(stage, func(t *testing.T) {
			entered := make(chan struct{}, 1)
			canceled := make(chan struct{}, 1)
			finish := make(chan struct{})
			played := make(chan string, 2)
			block := func(c context.Context) error {
				entered <- struct{}{}
				<-c.Done()
				canceled <- struct{}{}
				<-finish
				return c.Err()
			}
			m, err := New(context.Background(), Options{Synthesizer: synthFunc(func(c context.Context, s string) ([]byte, error) {
				if s == "A" && stage == "synthesis" {
					return nil, block(c)
				}
				return []byte(s), nil
			}), Player: playFunc(func(c context.Context, b []byte) error {
				if string(b) == "A" && stage == "playback" {
					return block(c)
				}
				played <- string(b)
				return nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			m.Enqueue("A")
			receive(t, entered)
			m.Enqueue("discard")
			stopped := make(chan error, 1)
			go func() { stopped <- m.Stop() }()
			receive(t, canceled)
			if !errors.Is(m.Enqueue("new"), ErrStopping) {
				t.Fatal("accepted while stopping")
			}
			close(finish)
			if err := receive(t, stopped); err != nil {
				t.Fatal(err)
			}
			if err = m.Enqueue("B"); err != nil {
				t.Fatal(err)
			}
			if receive(t, played) != "B" {
				t.Fatal("played cleared item")
			}
			if m.Snapshot().LastError != nil {
				t.Fatal("cancellation recorded")
			}
			m.Close()
			if !errors.Is(m.Enqueue(""), ErrClosed) {
				t.Fatal("closed precedence")
			}
		})
	}
}
func TestParentCancellationCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	released := make(chan struct{})
	m, err := New(ctx, Options{Synthesizer: synthFunc(func(c context.Context, s string) ([]byte, error) {
		close(entered)
		<-c.Done()
		close(released)
		return nil, c.Err()
	}), Player: playFunc(func(context.Context, []byte) error { t.Error("playback after cancellation"); return nil })})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	defer cancel()
	m.Enqueue("A")
	receive(t, entered)
	m.Enqueue("B")
	cancel()
	receive(t, released)
	receive(t, m.Done())
	if m.Snapshot().Pending != 0 || m.Snapshot().Phase != PhaseClosed || m.Snapshot().LastError != nil {
		t.Fatal(m.Snapshot())
	}
}

type closingSynth struct {
	entered  chan struct{}
	release  chan struct{}
	canceled chan struct{}
	closed   chan struct{}
	err      error
}

func (s *closingSynth) Synthesize(ctx context.Context, _ string) ([]byte, error) {
	close(s.entered)
	<-ctx.Done()
	close(s.canceled)
	<-s.release
	return nil, ctx.Err()
}

func (s *closingSynth) Close() error {
	close(s.closed)
	return s.err
}

type closingPlayer struct {
	closed chan struct{}
	err    error
}

func (p *closingPlayer) Play(context.Context, []byte) error { return nil }
func (p *closingPlayer) Close() error {
	close(p.closed)
	return p.err
}

func TestCloseWaitsBeforeReleasingSynthesizer(t *testing.T) {
	want := errors.New("close failure")
	playErr := errors.New("player close failure")
	player := &closingPlayer{closed: make(chan struct{}), err: playErr}
	synth := &closingSynth{
		entered: make(chan struct{}), release: make(chan struct{}),
		canceled: make(chan struct{}), closed: make(chan struct{}), err: want,
	}
	m, err := New(context.Background(), Options{
		Synthesizer: synth,
		Player:      player,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Enqueue("hello"); err != nil {
		t.Fatal(err)
	}
	receive(t, synth.entered)
	done := make(chan error, 1)
	go func() { done <- m.Close() }()
	receive(t, synth.canceled)
	select {
	case <-synth.closed:
		t.Fatal("synthesizer closed before synthesis returned")
	case <-player.closed:
		t.Fatal("player closed before synthesis returned")
	default:
	}
	close(synth.release)
	if err := receive(t, done); !errors.Is(err, want) || !errors.Is(err, playErr) {
		t.Fatalf("close error = %v", err)
	}
	receive(t, synth.closed)
	receive(t, player.closed)
	if err := m.Close(); !errors.Is(err, want) || !errors.Is(err, playErr) {
		t.Fatalf("repeated close error = %v", err)
	}
}

func TestSettingsMigrationAndProviderRetention(t *testing.T) {
	settings := DefaultSettings()
	if err := json.Unmarshal([]byte(`{"voice":"zh-CN-YunxiNeural","volume":63}`), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Provider != "edge" || settings.Values()["voice"] != "zh-CN-YunxiNeural" {
		t.Fatalf("legacy migration: %+v", settings)
	}
	original := settings
	settings.Provider = "melo"
	settings = settings.WithValue("threads", "4")
	normalized, err := settings.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Values()["backend"] != "webgpu" || normalized.Values()["speed"] != "1" || normalized.Providers["edge"]["voice"] != original.Values()["voice"] {
		t.Fatalf("provider settings lost: %+v", normalized)
	}
	if _, ok := original.Providers["melo"]; ok {
		t.Fatal("copy-on-write mutated original settings")
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["voice"]; ok {
		t.Fatal("legacy voice persisted")
	}
	reordered := Settings{Provider: "melo", Volume: 63, Providers: map[string]map[string]string{
		"melo": {"speed": "1", "threads": "4"},
	}}
	if normalized.Identity() != reordered.Identity() {
		t.Fatal("identity depends on inactive settings or map order")
	}
	cpu := normalized.WithValue("backend", "cpu")
	if cpu.Identity() == normalized.Identity() {
		t.Fatal("backend change did not change synthesizer identity")
	}
	encoded, err = json.Marshal(cpu)
	if err != nil {
		t.Fatal(err)
	}
	var restored Settings
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Values()["backend"] != "cpu" {
		t.Fatal("CPU backend was not retained after reload")
	}
}
