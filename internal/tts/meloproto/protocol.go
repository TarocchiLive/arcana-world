// Package meloproto 定义本地辅助进程的有界二进制协议，不依赖原生运行库。
package meloproto

import (
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	MaxText    = 2048
	MaxSeconds = 120
	MaxFrame   = 16 << 20 // 单帧载荷上限，读写两侧共用。
	Ready      = "local-tts-v1"
)

// SampleLimit 只接受随应用发布的三种模型采样率。
func SampleLimit(sampleRate int) int {
	switch sampleRate {
	case 44100, 24000, 22050:
		return sampleRate * MaxSeconds
	default:
		return 0
	}
}

// ValidWAV 验证 worker 的固定 PCM16 布局及所选模型的时长上限。
func ValidWAV(data []byte, sampleRate int) bool {
	limit := SampleLimit(sampleRate)
	if limit == 0 || len(data) < 46 || len(data) > 44+2*limit || (len(data)-44)%2 != 0 {
		return false
	}
	return string(data[:4]) == "RIFF" && binary.LittleEndian.Uint32(data[4:]) == uint32(len(data)-8) &&
		string(data[8:16]) == "WAVEfmt " && binary.LittleEndian.Uint32(data[16:]) == 16 &&
		binary.LittleEndian.Uint16(data[20:]) == 1 && binary.LittleEndian.Uint16(data[22:]) == 1 &&
		binary.LittleEndian.Uint32(data[24:]) == uint32(sampleRate) &&
		binary.LittleEndian.Uint32(data[28:]) == uint32(sampleRate*2) &&
		binary.LittleEndian.Uint16(data[32:]) == 2 && binary.LittleEndian.Uint16(data[34:]) == 16 &&
		string(data[36:40]) == "data" && binary.LittleEndian.Uint32(data[40:]) == uint32(len(data)-44)
}

func ValidText(text string) bool {
	return len(text) > 0 && len(text) <= MaxText && utf8.ValidString(text) && !strings.ContainsRune(text, 0) && strings.TrimSpace(text) != ""
}

func Read(r io.Reader, limit int) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(header[:])
	if limit < 1 || n == 0 || uint64(n) > uint64(limit) {
		return nil, errors.New("local TTS: invalid frame length")
	}
	data := make([]byte, int(n))
	if _, err := io.ReadFull(r, data); err != nil {
		// 帧头之后数据中断是截断，不能当作正常的流结束。
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return data, nil
}

func Write(w io.Writer, data []byte) error {
	if len(data) == 0 || len(data) > MaxFrame {
		return errors.New("local TTS: invalid frame length")
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(data)))
	for _, p := range [][]byte{header[:], data} {
		for len(p) != 0 {
			n, err := w.Write(p)
			if err != nil {
				return err
			}
			if n == 0 {
				return io.ErrShortWrite
			}
			p = p[n:]
		}
	}
	return nil
}
