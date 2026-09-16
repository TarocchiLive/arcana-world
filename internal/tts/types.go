package tts

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	DefaultQueueCapacity = 5
	MaxTextRunes         = 500
	MaxTextBytes         = 4 << 10
	MaxAudioBytes        = 8 << 20
)

var (
	ErrQueueFull   = errors.New("tts: queue full")
	ErrStopping    = errors.New("tts: stopping")
	ErrClosed      = errors.New("tts: closed")
	ErrInvalidText = errors.New("tts: invalid text")
)

type Synthesizer interface {
	Synthesize(context.Context, string) ([]byte, error)
}
type Player interface {
	Play(context.Context, []byte) error
}
type Options struct {
	QueueCapacity int
	Synthesizer   Synthesizer
	Player        Player
}
type Phase string

const (
	PhaseIdle         Phase = "idle"
	PhaseSynthesizing Phase = "synthesizing"
	PhasePlaying      Phase = "playing"
	PhaseStopping     Phase = "stopping"
	PhaseClosed       Phase = "closed"
)

type Snapshot struct {
	Phase     Phase
	Pending   int
	Dropped   uint64
	LastError error
}

func validateText(text string) (string, error) {
	if len(text) > MaxTextBytes || !utf8.ValidString(text) {
		return "", ErrInvalidText
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > MaxTextRunes {
		return "", ErrInvalidText
	}
	for _, r := range text {
		if !(r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r <= 0xd7ff || r >= 0xe000 && r <= 0xfffd || r >= 0x10000 && r <= 0x10ffff) {
			return "", ErrInvalidText
		}
	}
	return text, nil
}
