//go:build tts_audio

package audio

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakePlayback struct {
	playing  bool
	buffered int
	err      error
	closeErr error
	closed   bool
}

func (p *fakePlayback) IsPlaying() bool   { return p.playing }
func (p *fakePlayback) BufferedSize() int { return p.buffered }
func (p *fakePlayback) Err() error        { return p.err }
func (p *fakePlayback) Close() error      { p.closed = true; return p.closeErr }

func TestDrainRequiresEOFAndEmptySoftwareBuffers(t *testing.T) {
	for _, tt := range []struct {
		name     string
		state    pcmState
		playing  bool
		buffered int
		want     bool
	}{
		{"decoder not finished", pcmState{bytes: 4}, false, 0, false},
		{"still playing", pcmState{bytes: 4, eof: true}, true, 0, false},
		{"buffer remains", pcmState{bytes: 4, eof: true}, false, 4, false},
		{"fully drained", pcmState{bytes: 4, eof: true}, false, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			player := &fakePlayback{playing: tt.playing, buffered: tt.buffered}
			got, err := playbackDrained(player, func() error { return nil }, tt.state)
			if err != nil || got != tt.want {
				t.Fatalf("drained=%v err=%v", got, err)
			}
		})
	}
}

func TestDrainRejectsErrorsAndEmptyPCM(t *testing.T) {
	broken := errors.New("device lost")
	for _, tt := range []struct {
		name      string
		playerErr error
		deviceErr error
		state     pcmState
	}{
		{"decoder", nil, nil, pcmState{bytes: 4, eof: true, err: broken}},
		{"player", broken, nil, pcmState{bytes: 4, eof: true}},
		{"device", nil, broken, pcmState{bytes: 4, eof: true}},
		{"empty", nil, nil, pcmState{eof: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			drained, err := playbackDrained(&fakePlayback{err: tt.playerErr}, func() error { return tt.deviceErr }, tt.state)
			if drained || err == nil {
				t.Fatalf("drained=%v err=%v", drained, err)
			}
		})
	}
}

func TestTailCancellationAndDeviceFailureClosePlayer(t *testing.T) {
	for _, deviceFailure := range []bool{false, true} {
		name := "cancellation"
		if deviceFailure {
			name = "device failure"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			want := context.Canceled
			if deviceFailure {
				want = errors.New("device disappeared during tail")
			}
			checks := 0
			deviceErr := func() error {
				checks++
				// 第二次检查发生在软件缓冲排空并开始尾部等待之后。
				if checks == 2 {
					if deviceFailure {
						return want
					}
					cancel()
				}
				return nil
			}
			player := &fakePlayback{}
			stream := &trackedPCM{state: pcmState{bytes: 4, eof: true}}
			err := waitPlayback(ctx, player, deviceErr, stream, time.Hour)
			if !errors.Is(err, want) {
				t.Fatalf("got %v; want %v", err, want)
			}
			if !player.closed {
				t.Fatal("player was not closed")
			}
		})
	}
}

func TestNaturalDrainPropagatesCloseFailure(t *testing.T) {
	want := errors.New("close failed")
	player := &fakePlayback{closeErr: want}
	stream := &trackedPCM{state: pcmState{bytes: 4, eof: true}}
	if err := waitPlayback(context.Background(), player, func() error { return nil }, stream, 0); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
	if !player.closed {
		t.Fatal("player not closed")
	}
}
