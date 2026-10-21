package rtmpsrt

import (
	"encoding/binary"
	"io"
	"sync"
)

// 映射只保留有界历史。确认后丢弃旧边界；无确认时逐级合并旧边界，不能随直播时长增长。
// 落在两个边界之间的确认保守映射到前一个完整输入边界，不确认尚未转发的原始字节。
type ackPoint struct{ wire, original uint64 }
type ackMap struct {
	mu               sync.Mutex
	points           [4096]ackPoint
	count            int
	wire             uint64
	remote, original uint64
}
type countedReader struct {
	io.Reader
	count uint64
}

func (r *countedReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.count += uint64(n)
	return n, err
}

type mappedWriter struct {
	io.Writer
	source   *countedReader
	mapping  *ackMap
	holding  bool
	original uint64
}

func (w *mappedWriter) hold() { w.holding = true }
func (w *mappedWriter) release() {
	w.holding = false
	w.original = w.source.count
	w.mapping.record(0, w.original)
}
func (w *mappedWriter) Write(p []byte) (int, error) {
	// 先发布边界再发送：本机 SRT listener 的 ACK 可能比 Write 返回更早到达。
	if !w.holding {
		w.original = w.source.count
	}
	w.mapping.record(uint64(len(p)), w.original)
	return w.Writer.Write(p)
}
func (m *ackMap) record(n, original uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.count == len(m.points) {
		// 均匀稀疏化旧历史，同时保留最新一半的精确边界；不丢弃零点与最后确认基准。
		for i := 1; i < 1024; i++ {
			m.points[i] = m.points[i*2]
		}
		copy(m.points[1024:], m.points[2048:])
		m.count = 3072
	}
	m.wire += n
	m.points[m.count] = ackPoint{m.wire, original}
	m.count++
}
func (m *ackMap) translate(sequence uint32) uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	candidate := (m.remote &^ uint64(0xffffffff)) | uint64(sequence)
	if candidate < m.remote && m.remote-candidate > 1<<31 {
		candidate += 1 << 32
	}
	if candidate < m.remote || candidate > m.wire {
		return uint32(m.original)
	}
	m.remote = candidate
	index := -1
	for i := 0; i < m.count && m.points[i].wire <= candidate; i++ {
		index = i
	}
	previous := ackPoint{}
	if index >= 0 {
		previous = m.points[index]
	}
	if index+1 < m.count {
		next := m.points[index+1]
		// 媒体载荷为恒等区间，必须精确映射区间内 ACK；小窗口不能等到整块 64KiB 才前进。
		if next.wire-previous.wire == next.original-previous.original {
			original := previous.original + candidate - previous.wire
			if original > m.original {
				m.original = original
			}
		}
	}
	if index >= 0 {
		if m.points[index].original > m.original {
			m.original = m.points[index].original
		}
		// 保留最后已确认点，下一次 ACK 落在相同区间时仍有稳定下界。
		copy(m.points[:], m.points[index:m.count])
		m.count -= index
	}
	return uint32(m.original)
}

type reverseState struct {
	length, received uint32
	kind             byte
	extended, known  bool
	control          [4]byte
	ack              [76]byte
	positions        [4]int
	ackLength        int
}

// relayReplies 保留服务器字节布局，仅原位修改 type 3 的四字节确认序号。
// 因此 OBS→服务器的 ACK 仍直接有效，不需要第二套累计偏移映射。
func relayReplies(dst io.Writer, src io.Reader, mapping *ackMap) error {
	if _, err := io.CopyN(dst, src, 3073); err != nil {
		return err
	}
	states := make(map[uint32]*reverseState)
	size := uint32(128)
	var header [18]byte
	buffer := make([]byte, 65536)
	for {
		if _, err := io.ReadFull(src, header[:1]); err != nil {
			return err
		}
		format := header[0] >> 6
		id := uint32(header[0] & 63)
		basic := 1
		if id == 0 {
			basic = 2
		} else if id == 1 {
			basic = 3
		}
		if _, err := io.ReadFull(src, header[1:basic]); err != nil {
			return err
		}
		if id == 0 {
			id = 64 + uint32(header[1])
		} else if id == 1 {
			id = 64 + uint32(binary.LittleEndian.Uint16(header[1:3]))
		}
		s := states[id]
		if s == nil {
			s = &reverseState{}
			states[id] = s
		}
		first := s.received == s.length
		if !s.known {
			if format != 0 {
				return errRTMP
			}
			first = true
		}
		if !first && format != 3 {
			return errRTMP
		}
		lengths := [4]int{11, 7, 3, 0}
		end := basic + lengths[format]
		if _, err := io.ReadFull(src, header[basic:end]); err != nil {
			return err
		}
		if format < 3 {
			s.extended = read24(header[basic:basic+3]) == 0xffffff
			if format < 2 {
				s.length = read24(header[basic+3 : basic+6])
				s.kind = header[basic+6]
			}
		}
		if s.extended {
			if _, err := io.ReadFull(src, header[end:end+4]); err != nil {
				return err
			}
			end += 4
		}
		if first {
			s.received = 0
			s.known = true
			s.ackLength = 0
		}
		ack := s.kind == 3
		if ack && s.length != 4 {
			return errRTMP
		}
		if ack {
			if s.ackLength+end > len(s.ack) {
				return errRTMP
			}
			copy(s.ack[s.ackLength:], header[:end])
			s.ackLength += end
		} else if err := writeAll(dst, header[:end]); err != nil {
			return err
		}
		remaining := min(size, s.length-s.received)
		for remaining > 0 {
			take := min(remaining, uint32(len(buffer)))
			if _, err := io.ReadFull(src, buffer[:take]); err != nil {
				return err
			}
			if (s.kind == 1 || s.kind == 2 || ack) && s.length == 4 {
				copy(s.control[s.received:], buffer[:take])
			}
			if ack {
				for i := range take {
					s.positions[s.received+i] = s.ackLength + int(i)
				}
				copy(s.ack[s.ackLength:], buffer[:take])
				s.ackLength += int(take)
			} else if err := writeAll(dst, buffer[:take]); err != nil {
				return err
			}
			s.received += take
			remaining -= take
		}
		if s.received != s.length {
			continue
		}
		if ack {
			binary.BigEndian.PutUint32(s.control[:], mapping.translate(binary.BigEndian.Uint32(s.control[:])))
			for i, position := range s.positions {
				s.ack[position] = s.control[i]
			}
			if err := writeAll(dst, s.ack[:s.ackLength]); err != nil {
				return err
			}
		}
		if s.kind == 1 {
			if s.length != 4 {
				return errRTMP
			}
			size = binary.BigEndian.Uint32(s.control[:])
			if size == 0 || size > 0x7fffffff {
				return errRTMP
			}
		}
		if s.kind == 2 {
			if s.length != 4 {
				return errRTMP
			}
			if aborted := states[binary.BigEndian.Uint32(s.control[:])]; aborted != nil {
				aborted.received = aborted.length
				aborted.ackLength = 0
			}
		}
	}
}
