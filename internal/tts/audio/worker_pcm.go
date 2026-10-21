//go:build tts

package audio

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"math"

	"github.com/ebitengine/oto/v3"
)

const workerFilterPhases = 1024

// workerPCM 将输入流转换为设备固定的双声道浮点 PCM。
// 采样率不同时使用 Blackman 窗 sinc 低通重采样；降采样时同步降低截止频率并加宽核。
// 预计算有限相位表，播放期间只执行乘加；环形缓冲不随音频时长增长。
type workerPCM struct {
	decoder          *decodedAudio
	input            *bufio.Reader
	frames           [256][2]float32
	loaded, position int64
	ended            bool
	radius           int
	weights          []float64
	pending          [8]byte
	offset, size     int
}

func newWorkerPCM(decoder *decodedAudio) *workerPCM {
	r := &workerPCM{decoder: decoder, input: bufio.NewReaderSize(decoder.source, 8192)}
	if decoder.rate == workerSampleRate {
		return r
	}
	// 截止点为两侧较低奈奎斯特频率的90%，为有限窗留下过渡带。
	// 最大输入192kHz时半径128；256帧足以覆盖完整滤波核。
	ratio := math.Max(1, float64(decoder.rate)/workerSampleRate)
	r.radius = int(math.Ceil(32 * ratio))
	cutoff := 0.9 / ratio
	taps := 2 * r.radius
	r.weights = make([]float64, workerFilterPhases*taps)
	for phase := range workerFilterPhases {
		fraction := float64(phase) / workerFilterPhases
		weights := r.weights[phase*taps : (phase+1)*taps]
		var sum float64
		for tap := range taps {
			x := float64(tap-r.radius+1) - fraction
			window := 0.42 + 0.5*math.Cos(math.Pi*x/float64(r.radius)) + 0.08*math.Cos(2*math.Pi*x/float64(r.radius))
			weight := cutoff
			if x != 0 {
				weight = math.Sin(math.Pi*cutoff*x) / (math.Pi * x)
			}
			weights[tap] = weight * window
			sum += weights[tap]
		}
		// 每相位归一化，保证稳态直流增益为1。
		for tap := range weights {
			weights[tap] /= sum
		}
	}
	return r
}

func (r *workerPCM) frame() ([2]float32, error) {
	var result [2]float32
	var data [8]byte
	n := r.decoder.channels * r.decoder.sampleBytes
	if _, err := io.ReadFull(r.input, data[:n]); err != nil {
		return result, err
	}
	for ch := range r.decoder.channels {
		p := data[ch*r.decoder.sampleBytes:]
		if r.decoder.format == oto.FormatSignedInt16LE {
			result[ch] = float32(int16(binary.LittleEndian.Uint16(p))) / 32768
		} else {
			result[ch] = math.Float32frombits(binary.LittleEndian.Uint32(p))
			if math.IsNaN(float64(result[ch])) || math.IsInf(float64(result[ch]), 0) {
				return result, errors.New("non-finite PCM sample")
			}
		}
	}
	if r.decoder.channels == 1 {
		result[1] = result[0]
	}
	return result, nil
}

func (r *workerPCM) next() error {
	var sample [2]float32
	if r.radius == 0 {
		// 同采样率不经过滤波器，保留输入样本。
		var err error
		sample, err = r.frame()
		if err != nil {
			return err
		}
	} else {
		center := r.position / workerSampleRate
		last := center + int64(r.radius)
		for !r.ended && r.loaded <= last {
			frame, err := r.frame()
			if err == io.EOF {
				r.ended = true
				break
			}
			if err != nil {
				return err
			}
			r.frames[r.loaded%int64(len(r.frames))] = frame
			r.loaded++
		}
		// 首尾以零延拓，但不输出额外滤波尾巴；总长为ceil(N*out/in)。
		if r.ended && center >= r.loaded {
			return io.EOF
		}
		phase := int(r.position%workerSampleRate) * workerFilterPhases / workerSampleRate
		taps := 2 * r.radius
		weights := r.weights[phase*taps : (phase+1)*taps]
		var sum [2]float64
		for tap, weight := range weights {
			index := center + int64(tap-r.radius+1)
			if index < 0 || index >= r.loaded {
				continue
			}
			frame := r.frames[index%int64(len(r.frames))]
			for ch := range sum {
				sum[ch] += float64(frame[ch]) * weight
			}
		}
		for ch := range sample {
			sample[ch] = float32(sum[ch])
		}
		r.position += int64(r.decoder.rate)
	}
	for ch := range sample {
		binary.LittleEndian.PutUint32(r.pending[ch*4:], math.Float32bits(sample[ch]))
	}
	r.offset, r.size = 0, len(r.pending)
	return nil
}

func (r *workerPCM) Read(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		if r.offset == r.size {
			if err := r.next(); err != nil {
				return written, err
			}
		}
		n := copy(p, r.pending[r.offset:r.size])
		r.offset += n
		written += n
		p = p[n:]
	}
	return written, nil
}
