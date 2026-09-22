//go:build tts_audio

package audio

import (
	"bytes"
	"errors"
	"io"
	"sync"

	"github.com/hajimehoshi/go-mp3"
)

const (
	sampleRate        = 24000
	pcmChannels       = 2
	pcmBytesPerSample = 2 // go-mp3 提供的有符号 16 位 PCM。
	pcmBytesPerSecond = sampleRate * pcmChannels * pcmBytesPerSample
	maxAudioSeconds   = 120
	maxAudioBytes     = 8 << 20
	maxMetadataBytes  = 1 << 20
	maxPCMBytes       = pcmBytesPerSecond * maxAudioSeconds
)

func validateMP3(data []byte) error {
	if len(data) == 0 {
		return errors.New("audio: empty MP3")
	}
	if len(data) > maxAudioBytes {
		return errors.New("audio: MP3 exceeds 8 MiB")
	}
	if len(data) < 3 || string(data[:3]) != "ID3" {
		return nil
	}
	if len(data) < 10 {
		return errors.New("audio: truncated ID3 header")
	}
	size := 0
	for _, b := range data[6:10] {
		if b&0x80 != 0 {
			return errors.New("audio: invalid ID3 length")
		}
		size = size<<7 | int(b)
	}
	if size > maxMetadataBytes || size > len(data)-10 {
		return errors.New("audio: ID3 metadata exceeds source bounds or 1 MiB")
	}
	return nil
}

// readerOnly 防止解码器为支持定位而对整个 MP3 建立索引。
type readerOnly struct{ io.Reader }

func decodeMP3(data []byte) (*mp3.Decoder, error) {
	if err := validateMP3(data); err != nil {
		return nil, err
	}
	decoder, err := mp3.NewDecoder(readerOnly{bytes.NewReader(data)})
	if err != nil {
		return nil, errors.New("audio: invalid MP3 stream")
	}
	if decoder.SampleRate() != sampleRate {
		return nil, errors.New("audio: MP3 sample rate must be 24000 Hz")
	}
	return decoder, nil
}

type pcmState struct {
	bytes int
	eof   bool
	err   error
}

type trackedPCM struct {
	source io.Reader
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
	if len(p) == 0 {
		return 0, nil
	}
	if r.state.err != nil {
		return 0, r.state.err
	}
	if r.state.eof {
		return 0, io.EOF
	}
	remaining := maxPCMBytes - r.state.bytes
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
