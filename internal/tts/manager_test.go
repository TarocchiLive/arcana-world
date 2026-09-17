package tts

import (
	"context"
	"errors"
	"strings"
	"sync"
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
func TestSerialQueue(t *testing.T) {
	starts := make(chan string, 10)
	plays := make(chan string, 10)
	release := make(chan struct{})
	m, err := New(context.Background(), Options{Synthesizer: synthFunc(func(c context.Context, s string) ([]byte, error) { starts <- s; return []byte(s), nil }), Player: playFunc(func(c context.Context, b []byte) error {
		plays <- string(b)
		select {
		case <-release:
			return nil
		case <-c.Done():
			return c.Err()
		}
	})})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = m.Enqueue("A"); err != nil {
		t.Fatal(err)
	}
	if receive(t, starts) != "A" || receive(t, plays) != "A" {
		t.Fatal("wrong first item")
	}
	for _, s := range []string{"B", "C", "D", "E", "F"} {
		if err = m.Enqueue(s); err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(m.Enqueue("G"), ErrQueueFull) {
		t.Fatal("overflow accepted")
	}
	if s := m.Snapshot(); s.Pending != 5 || s.Dropped != 1 {
		t.Fatal(s)
	}
	select {
	case s := <-starts:
		t.Fatalf("prefetched %s", s)
	default:
	}
	for _, s := range []string{"B", "C", "D", "E", "F"} {
		release <- struct{}{}
		if got := receive(t, starts); got != s {
			t.Fatalf("got %s want %s", got, s)
		}
		if got := receive(t, plays); got != s {
			t.Fatal(got)
		}
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
func TestTextValidation(t *testing.T) {
	for _, s := range []string{"", "  ", strings.Repeat("你", 501), "\x00", "\xff", strings.Repeat(" ", MaxTextBytes+1)} {
		if _, err := validateText(s); !errors.Is(err, ErrInvalidText) {
			t.Fatalf("accepted invalid text")
		}
	}
	if _, err := validateText(strings.Repeat("你", 500)); err != nil {
		t.Fatal(err)
	}
}
func TestFailureContinues(t *testing.T) {
	failure := errors.New("failure")
	played := make(chan string, 2)
	m, _ := New(context.Background(), Options{Synthesizer: synthFunc(func(c context.Context, s string) ([]byte, error) {
		if s == "bad" {
			return nil, failure
		}
		return []byte(s), nil
	}), Player: playFunc(func(c context.Context, b []byte) error { played <- string(b); return nil })})
	defer m.Close()
	m.Enqueue("bad")
	m.Enqueue("good")
	if receive(t, played) != "good" {
		t.Fatal("wrong item")
	}
	if !errors.Is(m.Snapshot().LastError, failure) {
		t.Fatal("lost failure")
	}
}

func TestStopAtStageBoundary(t *testing.T) {
	for _, stage := range []string{"synthesis", "playback"} {
		t.Run(stage, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			canceled := make(chan struct{})
			starts := make(chan string, 8)
			plays := make(chan string, 8)
			boundary := func(c context.Context) { close(entered); <-c.Done(); close(canceled); <-release }
			m, err := New(context.Background(), Options{
				Synthesizer: synthFunc(func(c context.Context, s string) ([]byte, error) {
					starts <- s
					if stage == "synthesis" {
						boundary(c)
					}
					return []byte(s), nil
				}),
				Player: playFunc(func(c context.Context, b []byte) error {
					plays <- string(b)
					if stage == "playback" {
						boundary(c)
					}
					return nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			m.Enqueue("A")
			receive(t, entered)
			m.Enqueue("B")
			done := make(chan error, 1)
			go func() { done <- m.Stop() }()
			receive(t, canceled)
			close(release)
			receive(t, done)
			if receive(t, starts) != "A" {
				t.Fatal("wrong active text")
			}
			select {
			case next := <-starts:
				t.Fatalf("started waiting item %s", next)
			default:
			}
			if stage == "synthesis" {
				select {
				case <-plays:
					t.Fatal("started playback after stop")
				default:
				}
			}
		})
	}
}

func TestConcurrentCloseAndStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	cleanup := make(chan struct{})
	canceled := make(chan struct{})
	failure := errors.New("cleanup failed")
	m, err := New(ctx, Options{Synthesizer: synthFunc(func(c context.Context, s string) ([]byte, error) {
		close(entered)
		<-c.Done()
		close(canceled)
		<-cleanup
		return nil, failure
	}), Player: playFunc(func(context.Context, []byte) error { return nil })})
	if err != nil {
		t.Fatal(err)
	}
	m.Enqueue("A")
	receive(t, entered)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); m.Close() }()
	receive(t, canceled)
	for i := range 12 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			switch i % 3 {
			case 0:
				m.Enqueue("B")
			case 1:
				m.Stop()
			case 2:
				m.Close()
			}
		}(i)
	}
	cancel()
	close(cleanup)
	joined := make(chan struct{})
	go func() { wg.Wait(); close(joined) }()
	receive(t, joined)
	receive(t, m.Done())
	if !errors.Is(m.Close(), failure) {
		t.Fatal("lost shutdown error")
	}
	if m.Snapshot().Phase != PhaseClosed || !errors.Is(m.Enqueue(""), ErrClosed) {
		t.Fatal("reopened manager")
	}
}

func TestPlaybackFailureContinues(t *testing.T) {
	failure := errors.New("device failed")
	played := make(chan string, 3)
	m, err := New(context.Background(), Options{Synthesizer: synthFunc(func(c context.Context, s string) ([]byte, error) { return []byte(s), nil }), Player: playFunc(func(c context.Context, b []byte) error {
		played <- string(b)
		if string(b) == "bad" {
			return failure
		}
		return nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.Enqueue("bad")
	m.Enqueue("good")
	if receive(t, played) != "bad" || receive(t, played) != "good" {
		t.Fatal("retried or reordered playback")
	}
	if !errors.Is(m.Snapshot().LastError, failure) {
		t.Fatal("lost playback error")
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

func TestStopReturnsCleanupFailure(t *testing.T) {
	failure := errors.New("cleanup failure")
	entered := make(chan struct{})
	m, err := New(context.Background(), Options{Synthesizer: synthFunc(func(c context.Context, s string) ([]byte, error) {
		close(entered)
		<-c.Done()
		return nil, errors.Join(c.Err(), failure)
	}), Player: playFunc(func(context.Context, []byte) error { return nil })})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.Enqueue("A")
	receive(t, entered)
	if !errors.Is(m.Stop(), failure) {
		t.Fatal("lost cleanup failure")
	}
	if m.Snapshot().Phase != PhaseIdle {
		t.Fatal("stop did not finish")
	}
}
