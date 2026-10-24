//go:build tts

package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"

	"github.com/ebitengine/oto/v3"
)

func decodeWAV(data []byte) (*decodedAudio, error) {
	invalid := errors.New("audio: invalid WAV stream")
	if len(data) < 12 || string(data[8:12]) != "WAVE" {
		return nil, invalid
	}
	// RIFF 大小为 0、0xFFFFFFFF（流式写入）或超出实际长度时，以实际长度为准；
	// 小于实际长度时忽略其后的多余字节。
	if riff := uint64(binary.LittleEndian.Uint32(data[4:])) + 8; riff >= 12 && riff < uint64(len(data)) {
		data = data[:riff]
	}
	var format, pcm []byte
	truncated := false
	for offset := 12; len(data)-offset >= 8; {
		size := uint64(binary.LittleEndian.Uint32(data[offset+4:]))
		start := offset + 8
		id := string(data[offset : offset+4])
		if size > uint64(len(data)-start) {
			// 只有 data 块允许截断或使用流式占位大小，按实际存在的字节处理。
			if id != "data" || pcm != nil {
				return nil, invalid
			}
			size, truncated = uint64(len(data)-start), true
		}
		end := start + int(size)
		switch id {
		case "fmt ":
			if format != nil {
				return nil, invalid
			}
			format = data[start:end]
		case "data":
			if pcm != nil {
				return nil, invalid
			}
			pcm = data[start:end]
		}
		offset = end + int(size&1)
	}
	if len(format) < 16 || len(pcm) == 0 {
		return nil, invalid
	}
	encoding := binary.LittleEndian.Uint16(format)
	channels := int(binary.LittleEndian.Uint16(format[2:]))
	rate := int(binary.LittleEndian.Uint32(format[4:]))
	block := int(binary.LittleEndian.Uint16(format[12:]))
	bits := int(binary.LittleEndian.Uint16(format[14:]))
	if err := validLayout(rate, channels); err != nil {
		return nil, err
	}
	if encoding == 0xfffe {
		if len(format) < 40 || binary.LittleEndian.Uint16(format[16:]) < 22 || int(binary.LittleEndian.Uint16(format[18:])) != bits {
			return nil, errors.New("audio: unsupported WAV extensible valid bit depth")
		}
		guidTail := []byte{0, 0, 0, 0, 0x10, 0, 0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71}
		if !bytes.Equal(format[26:40], guidTail) {
			return nil, errors.New("audio: unsupported WAV subformat")
		}
		encoding = binary.LittleEndian.Uint16(format[24:])
		mask := binary.LittleEndian.Uint32(format[20:])
		if mask != 0 && !((channels == 1 && mask == 4) || (channels == 2 && mask == 3)) {
			return nil, errors.New("audio: unsupported WAV channel mask")
		}
	}
	if (encoding != 1 || (bits != 8 && bits != 16 && bits != 24 && bits != 32)) && (encoding != 3 || (bits != 32 && bits != 64)) {
		return nil, errors.New("audio: unsupported WAV encoding (PCM 8/16/24/32 or float 32/64 required)")
	}
	if truncated {
		// 末尾不足一个完整采样帧的残余字节直接丢弃。
		pcm = pcm[:len(pcm)/max(block, 1)*max(block, 1)]
	}
	if block != channels*(bits/8) || uint64(binary.LittleEndian.Uint32(format[8:])) != uint64(rate*block) || len(pcm)%block != 0 || len(pcm) == 0 {
		return nil, invalid
	}
	if len(pcm)/block > rate*maxAudioSeconds {
		return nil, errors.New("audio: decoded PCM exceeds 120 seconds")
	}
	if encoding == 1 && bits == 16 {
		return &decodedAudio{bytes.NewReader(pcm), rate, channels, 2, oto.FormatSignedInt16LE}, nil
	}
	return floatAudio(&wavSamples{data: pcm, bits: bits, floating: encoding == 3}, rate, channels), nil
}

type wavSamples struct {
	data     []byte
	bits     int
	floating bool
}

func (r *wavSamples) Read(p []float32) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	width := r.bits / 8
	count := min(len(p), len(r.data)/width)
	for i := range count {
		b := r.data[i*width:]
		var sample float64
		if r.floating {
			if r.bits == 32 {
				sample = float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
			} else {
				sample = math.Float64frombits(binary.LittleEndian.Uint64(b))
			}
			if math.IsNaN(sample) || math.IsInf(sample, 0) {
				return 0, errors.New("non-finite WAV sample")
			}
			sample = max(-1, min(1, sample))
		} else {
			switch r.bits {
			case 8:
				sample = float64(int(b[0])-128) / 128
			case 24:
				sample = float64(int32(uint32(b[0])<<8|uint32(b[1])<<16|uint32(b[2])<<24)>>8) / 8388608
			case 32:
				sample = float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
			}
		}
		p[i] = float32(sample)
	}
	r.data = r.data[count*width:]
	return count, nil
}
