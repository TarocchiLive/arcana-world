//go:build tts

package audio

import (
	"context"
	"errors"
	"io"
	"sync"
)

const (
	maxAudioSeconds = 120
	maxAudioBytes   = 16 << 20
)

type pcmState struct {
	bytes int
	eof   bool
	err   error
}

type trackedPCM struct {
	source io.Reader
	ctx    context.Context
	limit  int
	mu     sync.Mutex
	state  pcmState
}

func (r *trackedPCM) snapshot() pcmState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

func (r *trackedPCM) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ctx.Err(); err != nil {
		r.state.err = err
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.state.err != nil {
		return 0, r.state.err
	}
	if r.state.eof {
		return 0, io.EOF
	}
	remaining := r.limit - r.state.bytes
	if remaining == 0 {
		var probe [1]byte
		n, err := r.source.Read(probe[:])
		if n != 0 {
			r.state.err = errors.New("audio: decoded PCM exceeds 120 seconds")
			return 0, r.state.err
		}
		if err == io.EOF {
			r.state.eof = true
		} else if err != nil {
			r.state.err = err
		}
		return 0, err
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err := r.source.Read(p)
	r.state.bytes += n
	if err == io.EOF {
		r.state.eof = true
	} else if err != nil {
		r.state.err = err
	}
	return n, err
}
