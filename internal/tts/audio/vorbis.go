//go:build tts

package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/bits"
)

func boundedVorbisComments(packet []byte) bool {
	if len(packet) < 16 || !bytes.Equal(packet[:7], []byte("\x03vorbis")) {
		return false
	}
	packet = packet[7:]
	vendor := uint64(binary.LittleEndian.Uint32(packet))
	if vendor > uint64(len(packet)-4) {
		return false
	}
	packet = packet[4+int(vendor):]
	if len(packet) < 5 {
		return false
	}
	count := uint64(binary.LittleEndian.Uint32(packet))
	packet = packet[4:]
	if count > uint64((len(packet)-1)/4) {
		return false
	}
	for range count {
		if len(packet) < 4 {
			return false
		}
		size := uint64(binary.LittleEndian.Uint32(packet))
		if size > uint64(len(packet)-4) {
			return false
		}
		packet = packet[4+int(size):]
	}
	return len(packet) > 0 && packet[0]&1 != 0
}

type vorbisBits struct {
	data   []byte
	pos    uint64
	failed bool
}

func (b *vorbisBits) read(n uint) uint32 {
	if b.failed || n > 32 || uint64(n) > uint64(len(b.data))*8-b.pos {
		b.failed = true
		return 0
	}
	var value uint32
	for shift := uint(0); shift < n; {
		take := min(n-shift, 8-uint(b.pos%8))
		value |= uint32(b.data[b.pos/8]>>uint(b.pos%8)) & ((1 << take) - 1) << shift
		b.pos += uint64(take)
		shift += take
	}
	return value
}

func (b *vorbisBits) skip(n uint64) {
	if b.failed || n > uint64(len(b.data))*8-b.pos {
		b.failed = true
		return
	}
	b.pos += n
}

// 先限制第三方解码器按头部字段分配的内存和循环次数；仅限制包长无法阻止过量分配。
func boundedVorbisSetup(packet []byte, channels int) bool {
	if len(packet) < 8 || !bytes.Equal(packet[:7], []byte("\x05vorbis")) {
		return false
	}
	b := vorbisBits{data: packet[7:]}
	bookCount := int(b.read(8)) + 1
	var vectorBooks [256]bool
	budget := uint64(32 << 20)
	for book := range bookCount {
		if b.read(24) != 0x564342 {
			return false
		}
		dimensions, entries := b.read(16), b.read(24)
		cost := uint64(entries) * 9
		if dimensions == 0 || entries == 0 || cost > budget {
			return false
		}
		budget -= cost
		if b.read(1) == 0 {
			sparse := b.read(1) != 0
			for i := uint32(0); i < entries && !b.failed; i++ {
				if !sparse || b.read(1) != 0 {
					b.skip(5)
				}
			}
		} else {
			length := b.read(5) + 1
			for current := uint32(0); current < entries; length++ {
				if b.failed || length > 32 {
					return false
				}
				n := b.read(uint(bits.Len32(entries - current)))
				if n > entries-current {
					return false
				}
				current += n
			}
		}
		lookup := b.read(4)
		if b.failed || lookup > 2 {
			return false
		}
		if lookup == 0 {
			continue
		}
		vectorBooks[book] = true
		b.skip(64)
		valueBits := b.read(4) + 1
		b.skip(1)
		values := uint64(entries) * uint64(dimensions)
		multiplicands := values
		if lookup == 1 {
			multiplicands = uint64(math.Floor(math.Pow(float64(entries), 1/float64(dimensions))))
		}
		cost = 4 * (values + multiplicands)
		if cost > budget {
			return false
		}
		budget -= cost
		b.skip(multiplicands * uint64(valueBits))
	}
	for times := int(b.read(6)) + 1; times > 0; times-- {
		if b.read(16) != 0 {
			return false
		}
	}
	floorCount := int(b.read(6)) + 1
	for range floorCount {
		switch b.read(16) {
		case 0:
			order, rate, bark := b.read(8), b.read(16), b.read(16)
			if order == 0 || rate == 0 || bark == 0 {
				return false
			}
			b.skip(14)
			for books := int(b.read(4)) + 1; books > 0; books-- {
				book := int(b.read(8))
				if book >= bookCount || !vectorBooks[book] {
					return false
				}
			}
		case 1:
			partitions := int(b.read(5))
			var classes [32]uint32
			var dimensions [16]uint32
			maxClass := -1
			for i := range partitions {
				classes[i] = b.read(4)
				maxClass = max(maxClass, int(classes[i]))
			}
			for class := 0; class <= maxClass; class++ {
				dimensions[class] = b.read(3) + 1
				subclasses := b.read(2)
				if subclasses != 0 && int(b.read(8)) >= bookCount {
					return false
				}
				for range 1 << subclasses {
					if int(b.read(8)) > bookCount {
						return false
					}
				}
			}
			b.skip(2)
			rangeBits := b.read(4)
			for i := range partitions {
				b.skip(uint64(dimensions[classes[i]] * rangeBits))
			}
		default:
			return false
		}
	}
	residueCount := int(b.read(6)) + 1
	for range residueCount {
		if b.read(16) > 2 {
			return false
		}
		begin, end := b.read(24), b.read(24)
		if begin > end {
			return false
		}
		b.skip(24)
		classifications := int(b.read(6)) + 1
		if int(b.read(8)) >= bookCount {
			return false
		}
		var cascades [64]uint32
		for i := range classifications {
			cascades[i] = b.read(3)
			if b.read(1) != 0 {
				cascades[i] |= b.read(5) << 3
			}
		}
		for i := range classifications {
			for bit := range 8 {
				if cascades[i]&(1<<bit) != 0 {
					book := int(b.read(8))
					if book >= bookCount || !vectorBooks[book] {
						return false
					}
				}
			}
		}
	}
	mappingCount := int(b.read(6)) + 1
	for range mappingCount {
		if b.read(16) != 0 {
			return false
		}
		submaps := 1
		if b.read(1) != 0 {
			submaps = int(b.read(4)) + 1
		}
		if b.read(1) != 0 {
			for steps := int(b.read(8)) + 1; steps > 0; steps-- {
				width := uint(bits.Len32(uint32(channels - 1)))
				magnitude, angle := b.read(width), b.read(width)
				if magnitude == angle || magnitude >= uint32(channels) || angle >= uint32(channels) {
					return false
				}
			}
		}
		if b.read(2) != 0 {
			return false
		}
		if submaps > 1 {
			for range channels {
				if int(b.read(4)) >= submaps {
					return false
				}
			}
		}
		for range submaps {
			b.skip(8)
			if int(b.read(8)) >= floorCount || int(b.read(8)) >= residueCount {
				return false
			}
		}
	}
	for modes := int(b.read(6)) + 1; modes > 0; modes-- {
		b.skip(1)
		if b.read(16) != 0 || b.read(16) != 0 || int(b.read(8)) >= mappingCount {
			return false
		}
	}
	return b.read(1) == 1 && !b.failed
}
