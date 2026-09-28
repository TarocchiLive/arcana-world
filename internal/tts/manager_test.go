package tts

import (
	"context"
	"errors"
	"testing"
	"time"
)

type synthFunc func(context.Context, string) ([]byte, error)

func (f synthFunc) Synthesize(c context.Context, s string) ([]byte, error) { return f(c, s) }

type playFunc func(context.Context, []byte) error

func (f playFunc) Play(c context.Context, b []byte) error { return f(c, b) }
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
