package rtmpsrt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

var errRTMP = errors.New("invalid RTMP stream")

func read24(p []byte) uint32   { return uint32(p[0])<<16 | uint32(p[1])<<8 | uint32(p[2]) }
func put24(p []byte, n uint32) { p[0] = byte(n >> 16); p[1] = byte(n >> 8); p[2] = byte(n) }
func writeAll(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}

type chunkState struct {
	timestamp, delta, length, stream, received uint32
	kind                                       byte
	extended, known                            bool
	command                                    []byte
	control                                    [4]byte
}

// relayRTMP 保留握手与媒体字节，只重写 connect 的 app/tcUrl。
// 每条消息输出完整头，避免修改 connect 长度后压缩头继承错误长度。
func relayRTMP(dst io.Writer, src io.Reader, target Target) error {
	if _, err := io.CopyN(dst, src, 3073); err != nil {
		return err
	} // C0+C1+C2；反向泵独立转发 S0/S1/S2。
	states := make(map[uint32]*chunkState)
	chunkSize := uint32(128)
	connected := false
	buffer := make([]byte, 65536)
	var header [18]byte
	for {
		if _, err := io.ReadFull(src, header[:1]); err != nil {
			return err
		}
		format := header[0] >> 6
		csid := uint32(header[0] & 63)
		if csid == 0 {
			if _, err := io.ReadFull(src, header[:1]); err != nil {
				return err
			}
			csid = 64 + uint32(header[0])
		} else if csid == 1 {
			if _, err := io.ReadFull(src, header[:2]); err != nil {
				return err
			}
			csid = 64 + uint32(header[0]) + uint32(header[1])*256
		}
		s := states[csid]
		if s == nil {
			s = &chunkState{}
			states[csid] = s
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
		sizes := [4]int{11, 7, 3, 0}
		if _, err := io.ReadFull(src, header[:sizes[format]]); err != nil {
			return err
		}
		stamp := uint32(0)
		if format < 3 {
			stamp = read24(header[:3])
			s.extended = stamp == 0xffffff
			if format < 2 {
				s.length = read24(header[3:6])
				s.kind = header[6]
			}
			if format == 0 {
				s.stream = binary.LittleEndian.Uint32(header[7:11])
			}
		}
		if s.extended {
			if _, err := io.ReadFull(src, header[:4]); err != nil {
				return err
			}
			stamp = binary.BigEndian.Uint32(header[:4])
		}
		if first {
			s.received = 0
			s.known = true
			switch format {
			case 0:
				s.timestamp = stamp
				s.delta = stamp
			case 1, 2:
				s.delta = stamp
				s.timestamp += stamp
			case 3:
				s.timestamp += s.delta
			}
			if !connected && s.kind == 20 {
				if s.length > 65536 {
					return errRTMP
				}
				s.command = make([]byte, 0, s.length)
				if writer, ok := dst.(*mappedWriter); ok {
					writer.hold()
				}
			}
		}
		n := min(chunkSize, s.length-s.received)
		capture := !connected && s.kind == 20
		if !capture {
			if err := writeChunkHeader(dst, csid, s, s.length, first); err != nil {
				return err
			}
		}
		remaining := n
		for remaining > 0 {
			take := min(remaining, uint32(len(buffer)))
			if _, err := io.ReadFull(src, buffer[:take]); err != nil {
				return err
			}
			if capture {
				s.command = append(s.command, buffer[:take]...)
			} else {
				if err := writeAll(dst, buffer[:take]); err != nil {
					return err
				}
			}
			if (s.kind == 1 || s.kind == 2) && s.length == 4 {
				copy(s.control[s.received+(n-remaining):], buffer[:take])
			}
			remaining -= take
		}
		s.received += n
		if s.received == s.length {
			if capture {
				command, err := rewriteConnect(s.command, target)
				if err != nil {
					return err
				}
				connected = true
				for offset := 0; offset < len(command); {
					if err := writeChunkHeader(dst, csid, s, uint32(len(command)), offset == 0); err != nil {
						return err
					}
					end := min(offset+int(chunkSize), len(command))
					if err := writeAll(dst, command[offset:end]); err != nil {
						return err
					}
					offset = end
				}
				s.command = nil
				if writer, ok := dst.(*mappedWriter); ok {
					writer.release()
				}
			}
			if s.kind == 1 {
				if s.length != 4 {
					return errRTMP
				}
				chunkSize = binary.BigEndian.Uint32(s.control[:])
				if chunkSize == 0 || chunkSize > 0x7fffffff {
					return errRTMP
				}
			}
			if s.kind == 2 {
				if s.length != 4 {
					return errRTMP
				}
				if aborted := states[binary.BigEndian.Uint32(s.control[:])]; aborted != nil {
					aborted.received = aborted.length
					aborted.command = nil
				}
			}
		}
	}
}

func writeChunkHeader(w io.Writer, csid uint32, s *chunkState, length uint32, first bool) error {
	var b [18]byte
	format := byte(3)
	if first {
		format = 0
	}
	n := 1
	switch {
	case csid < 64:
		b[0] = format<<6 | byte(csid)
	case csid < 320:
		b[0] = format << 6
		b[1] = byte(csid - 64)
		n = 2
	default:
		b[0] = format<<6 | 1
		binary.LittleEndian.PutUint16(b[1:3], uint16(csid-64))
		n = 3
	}
	if first {
		put24(b[n:n+3], min(s.timestamp, uint32(0xffffff)))
		put24(b[n+3:n+6], length)
		b[n+6] = s.kind
		binary.LittleEndian.PutUint32(b[n+7:n+11], s.stream)
		n += 11
	}
	if s.timestamp >= 0xffffff {
		binary.BigEndian.PutUint32(b[n:n+4], s.timestamp)
		n += 4
	}
	return writeAll(w, b[:n])
}

func amfString(s string) ([]byte, error) {
	if len(s) > 65535 {
		return nil, errRTMP
	}
	out := make([]byte, 3+len(s))
	out[0] = 2
	binary.BigEndian.PutUint16(out[1:3], uint16(len(s)))
	copy(out[3:], s)
	return out, nil
}

// amfEnd 仅定位 AMF0 值边界；未知类型拒绝，避免在字符串内容中误替换 tcUrl。
func amfEnd(p []byte, start, depth int) (int, error) {
	if start >= len(p) || depth > 32 {
		return 0, errRTMP
	}
	n := start + 1
	switch p[start] {
	case 0:
		n += 8
	case 1:
		n++
	case 2:
		if n+2 > len(p) {
			return 0, errRTMP
		}
		n += 2 + int(binary.BigEndian.Uint16(p[n:n+2]))
	case 5, 6:
	case 3, 8:
		if p[start] == 8 {
			n += 4
		}
		for {
			if n+3 > len(p) {
				return 0, errRTMP
			}
			size := int(binary.BigEndian.Uint16(p[n : n+2]))
			n += 2
			if size == 0 && p[n] == 9 {
				n++
				break
			}
			n += size
			end, err := amfEnd(p, n, depth+1)
			if err != nil {
				return 0, errRTMP
			}
			n = end
		}
	case 10:
		if n+4 > len(p) {
			return 0, errRTMP
		}
		count := binary.BigEndian.Uint32(p[n : n+4])
		n += 4
		for ; count > 0; count-- {
			end, err := amfEnd(p, n, depth+1)
			if err != nil {
				return 0, errRTMP
			}
			n = end
		}
	case 11:
		n += 10
	case 12:
		if n+4 > len(p) {
			return 0, errRTMP
		}
		size := binary.BigEndian.Uint32(p[n : n+4])
		if uint64(size) > uint64(len(p)) {
			return 0, errRTMP
		}
		n += 4 + int(size)
	default:
		return 0, errRTMP
	}
	if n > len(p) {
		return 0, errRTMP
	}
	return n, nil
}
func rewriteConnect(p []byte, target Target) ([]byte, error) {
	if len(p) < 10 || !bytes.Equal(p[:10], []byte{2, 0, 7, 'c', 'o', 'n', 'n', 'e', 'c', 't'}) {
		return nil, errRTMP
	}
	n, err := amfEnd(p, 10, 0)
	if err != nil || n >= len(p) || p[n] != 3 {
		return nil, errRTMP
	}
	n++
	out := append([]byte(nil), p[:n]...)
	app, tc := false, false
	for {
		if n+3 > len(p) {
			return nil, errRTMP
		}
		size := int(binary.BigEndian.Uint16(p[n : n+2]))
		if size == 0 && p[n+2] == 9 {
			out = append(out, p[n:]...)
			break
		}
		endName := n + 2 + size
		if endName > len(p) {
			return nil, errRTMP
		}
		end, err := amfEnd(p, endName, 0)
		if err != nil {
			return nil, err
		}
		name := string(p[n+2 : endName])
		out = append(out, p[n:endName]...)
		value := ""
		replace := false
		if name == "app" {
			app = true
			value = target.App
			replace = true
		}
		if name == "tcUrl" {
			tc = true
			value = target.TCURL
			replace = true
		}
		if replace {
			encoded, err := amfString(value)
			if err != nil {
				return nil, err
			}
			out = append(out, encoded...)
		} else {
			out = append(out, p[endName:end]...)
		}
		n = end
	}
	if !app || !tc {
		return nil, errRTMP
	}
	return out, nil
}
