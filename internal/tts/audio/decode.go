//go:build tts

package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/ebitengine/oto/v3"
	"github.com/hajimehoshi/go-mp3"
	"github.com/jfreymuth/oggvorbis"
	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
)

type decodedAudio struct {
	source                      io.Reader
	rate, channels, sampleBytes int
	format                      oto.Format
}

func validLayout(rate, channels int) error {
	if rate < 8000 || rate > 192000 || (channels != 1 && channels != 2) {
		return errors.New("audio: unsupported sample rate or channel count (8000–192000 Hz, mono/stereo required)")
	}
	return nil
}

// readerOnly 避免解码器为了定位而预先扫描整个压缩流。
type readerOnly struct{ io.Reader }

func decodeAudio(data []byte) (*decodedAudio, error) {
	if len(data) == 0 {
		return nil, errors.New("audio: empty input")
	}
	if len(data) > maxAudioBytes {
		return nil, fmt.Errorf("audio: input exceeds limit (%d MiB)", maxAudioBytes>>20)
	}
	var result *decodedAudio
	switch {
	case bytes.HasPrefix(data, []byte("RIFF")):
		return decodeWAV(data)
	case bytes.HasPrefix(data, []byte("fLaC")):
		stream, err := flac.New(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("audio: invalid FLAC: %w", err)
		}
		info := stream.Info
		if err := validLayout(int(info.SampleRate), int(info.NChannels)); err != nil {
			return nil, err
		}
		// 当前解码库不支持32位FLAC；明确拒绝，不截断样本。
		if info.BitsPerSample < 8 || info.BitsPerSample > 24 {
			return nil, errors.New("audio: unsupported FLAC bit depth (8–24 required)")
		}
		if info.NSamples > uint64(info.SampleRate)*maxAudioSeconds {
			return nil, errors.New("audio: decoded PCM exceeds 120 seconds")
		}
		source := &flacSamples{stream: stream}
		result = floatAudio(source, int(info.SampleRate), int(info.NChannels))
	case bytes.HasPrefix(data, []byte("OggS")):
		if err := validateOgg(data); err != nil {
			return nil, err
		}
		stream, err := oggvorbis.NewReader(readerOnly{bytes.NewReader(data)})
		if err != nil {
			return nil, fmt.Errorf("audio: invalid Ogg Vorbis: %w", err)
		}
		result = floatAudio(stream, stream.SampleRate(), stream.Channels())
	default:
		payload, err := validateMP3(data)
		if err != nil {
			return nil, err
		}
		stream, err := mp3.NewDecoder(readerOnly{bytes.NewReader(payload)})
		if err != nil {
			return nil, fmt.Errorf("audio: invalid MP3 stream: %w", err)
		}
		// go-mp3将单声道复制为双声道，直接交给设备，避免再次转换。
		result = &decodedAudio{stream, stream.SampleRate(), 2, 2, oto.FormatSignedInt16LE}
	}
	if err := validLayout(result.rate, result.channels); err != nil {
		return nil, err
	}
	return result, nil
}

type floatSamples interface{ Read([]float32) (int, error) }

type floatPCM struct {
	source     floatSamples
	samples    [2048]float32
	pending    [8192]byte
	start, end int
	err        error
}

func floatAudio(source floatSamples, rate, channels int) *decodedAudio {
	return &decodedAudio{&floatPCM{source: source}, rate, channels, 4, oto.FormatFloat32LE}
}

// 固定小缓冲仅转换当前一批样本，不复制完整PCM，也不重采样。
func (r *floatPCM) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.start == r.end {
		if r.err != nil {
			return 0, r.err
		}
		n, err := r.source.Read(r.samples[:])
		r.err = err
		r.start, r.end = 0, n*4
		for i, sample := range r.samples[:n] {
			if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
				r.end = 0
				r.err = errors.New("non-finite PCM sample")
				return 0, r.err
			}
			if sample > 1 {
				sample = 1
			}
			if sample < -1 {
				sample = -1
			}
			binary.LittleEndian.PutUint32(r.pending[i*4:], math.Float32bits(sample))
		}
	}
	n := copy(p, r.pending[r.start:r.end])
	r.start += n
	if r.start == r.end {
		return n, r.err
	}
	return n, nil
}

type flacSamples struct {
	stream *flac.Stream
	frame  *frame.Frame
	offset int
	total  uint64
}

func (r *flacSamples) Read(p []float32) (int, error) {
	info := r.stream.Info
	channels := int(info.NChannels)
	if r.frame == nil || r.offset == len(r.frame.Subframes[0].Samples) {
		next, err := r.stream.Next()
		if err != nil {
			if err == io.EOF && info.NSamples != 0 && r.total != info.NSamples {
				return 0, io.ErrUnexpectedEOF
			}
			return 0, err
		}
		if next.BitsPerSample == 0 {
			next.BitsPerSample = info.BitsPerSample
		}
		if next.SampleRate == 0 {
			next.SampleRate = info.SampleRate
		}
		if next.Channels.Count() != channels || next.SampleRate != info.SampleRate || next.BitsPerSample != info.BitsPerSample {
			return 0, errors.New("FLAC frame layout changed")
		}
		if err := next.Parse(); err != nil {
			return 0, err
		}
		count := len(next.Subframes[0].Samples)
		for _, sub := range next.Subframes {
			if len(sub.Samples) != count {
				return 0, errors.New("invalid FLAC channel length")
			}
		}
		r.total += uint64(count)
		if r.total > uint64(info.SampleRate)*maxAudioSeconds || (info.NSamples != 0 && r.total > info.NSamples) {
			return 0, errors.New("FLAC sample count exceeds bounds")
		}
		r.frame, r.offset = next, 0
	}
	count := min(len(p)/channels, len(r.frame.Subframes[0].Samples)-r.offset)
	scale := float32(uint32(1) << (info.BitsPerSample - 1))
	for i := range count {
		for ch := range channels {
			p[i*channels+ch] = float32(r.frame.Subframes[ch].Samples[r.offset+i]) / scale
		}
	}
	r.offset += count
	return count * channels, nil
}
