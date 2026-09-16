//go:build tts_audio

package audio

import (
	"bytes"
	_ "embed"
	"errors"
	"io"
	"testing"
)

//go:embed testdata/24000.mp3
var mp3At24000 []byte

//go:embed testdata/44100.mp3
var mp3At44100 []byte

func TestRejectUnsafeMP3(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":              nil,
		"oversized":          make([]byte, maxAudioBytes+1),
		"truncated tag":      []byte("ID3\x04"),
		"invalid synchsafe":  {'I', 'D', '3', 4, 0, 0, 0x80, 0, 0, 0},
		"tag past end":       {'I', 'D', '3', 4, 0, 0, 0, 0, 0, 10},
		"excessive metadata": append([]byte{'I', 'D', '3', 4, 0, 0, 0, 0x40, 0, 1}, make([]byte, maxMetadataBytes+1)...),
		"wrong sample rate":  mp3At44100,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeMP3(data); err == nil {
				t.Fatal("unsafe MP3 accepted")
			}
		})
	}
}

func TestDecodeMonoMP3ToStereoPCM(t *testing.T) {
	decoder, err := decodeMP3(mp3At24000)
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := io.ReadAll(decoder)
	if err != nil {
		t.Fatal(err)
	}
	if len(pcm) == 0 || len(pcm)%4 != 0 {
		t.Fatalf("invalid stereo PCM size %d", len(pcm))
	}
	signal := false
	for i := 0; i < len(pcm); i += 4 {
		if !bytes.Equal(pcm[i:i+2], pcm[i+2:i+4]) {
			t.Fatal("mono source was not duplicated into stereo")
		}
		signal = signal || pcm[i] != 0 || pcm[i+1] != 0
	}
	if !signal {
		t.Fatal("fixture decoded to silence")
	}
}

func TestPCMLimitProbesEOF(t *testing.T) {
	exact := &trackedPCM{source: bytes.NewReader(nil), state: pcmState{bytes: maxPCMBytes}}
	var p [4]byte
	if n, err := exact.Read(p[:]); n != 0 || err != io.EOF || !exact.snapshot().eof {
		t.Fatalf("exact boundary: n=%d err=%v", n, err)
	}
	overflow := &trackedPCM{source: bytes.NewReader([]byte{1}), state: pcmState{bytes: maxPCMBytes}}
	if n, err := overflow.Read(p[:]); n != 0 || err == nil || err == io.EOF {
		t.Fatalf("overflow: n=%d err=%v", n, err)
	}
	if overflow.snapshot().err == nil {
		t.Fatal("overflow not retained for playback observer")
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestPCMDecodeFailureRetained(t *testing.T) {
	want := errors.New("broken frame")
	stream := &trackedPCM{source: failingReader{want}}
	var p [4]byte
	_, _ = stream.Read(p[:])
	if !errors.Is(stream.snapshot().err, want) {
		t.Fatal("decode error lost")
	}
}
