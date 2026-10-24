//go:build tts

package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// validateMP3 按帧校验真实采样率与声道；允许CBR/VBR，拒绝中途变更格式。
func validateMP3(data []byte) ([]byte, error) {
	if bytes.HasPrefix(data, []byte("ID3")) {
		if len(data) < 10 {
			return nil, errors.New("audio: invalid MP3 stream: truncated ID3")
		}
		if data[3] < 2 || data[3] > 4 {
			return nil, errors.New("audio: invalid MP3 stream: unsupported ID3 version")
		}
		size := 0
		for _, b := range data[6:10] {
			if b&128 != 0 {
				return nil, errors.New("audio: invalid MP3 stream: invalid ID3 length")
			}
			size = size<<7 | int(b)
		}
		size += 10
		if data[3] == 4 && data[5]&16 != 0 {
			size += 10
		}
		if size > 1<<20 || size > len(data) {
			return nil, errors.New("audio: invalid MP3 stream: ID3 exceeds bounds")
		}
		data = data[size:]
	}
	if len(data) >= 128 && string(data[len(data)-128:len(data)-125]) == "TAG" {
		data = data[:len(data)-128]
	}
	payload := data
	rate, channels, samples := 0, 0, 0
	// 至少有一个有效帧后，零填充、APE 标签或末尾不完整的帧都视为尾部数据并忽略；
	// 第一个有效帧之前的杂散数据仍然拒绝。
	for len(data) > 0 {
		if len(data) < 4 {
			if rate != 0 {
				break
			}
			return nil, errors.New("audio: invalid MP3 stream: truncated frame")
		}
		h := binary.BigEndian.Uint32(data)
		version, layer := (h>>19)&3, (h>>17)&3
		bitrateIndex, rateIndex := int((h>>12)&15), int((h>>10)&3)
		if h>>21 != 0x7ff || (version != 3 && version != 2) || layer != 1 || bitrateIndex == 0 || bitrateIndex == 15 || rateIndex == 3 {
			if rate != 0 {
				break
			}
			return nil, errors.New("audio: unsupported format (MP3 MPEG-1/2 Layer III, WAV, FLAC or Ogg Vorbis required)")
		}
		rates := [...]int{44100, 48000, 32000}
		currentRate := rates[rateIndex]
		bitrates := [...]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
		coefficient, frameSamples := 144000, 1152
		if version == 2 {
			currentRate /= 2
			bitrates = [...]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
			coefficient, frameSamples = 72000, 576
		}
		currentChannels := 2
		if (h>>6)&3 == 3 {
			currentChannels = 1
		}
		if rate != 0 && (rate != currentRate || channels != currentChannels) {
			return nil, errors.New("audio: invalid MP3 stream: frame layout changed")
		}
		size := coefficient*bitrates[bitrateIndex]/currentRate + int((h>>9)&1)
		if size > len(data) {
			if rate != 0 {
				break
			}
			return nil, errors.New("audio: invalid MP3 stream: truncated frame")
		}
		rate, channels = currentRate, currentChannels
		samples += frameSamples
		if samples > rate*maxAudioSeconds {
			return nil, errors.New("audio: decoded PCM exceeds 120 seconds")
		}
		data = data[size:]
	}
	if rate == 0 {
		return nil, errors.New("audio: invalid MP3 stream: no frames")
	}
	return payload[:len(payload)-len(data)], nil
}

// 限定单个逻辑 Vorbis 流，并在解码器分配内存前检查三个头包。
func validateOgg(data []byte) error {
	var serial uint32
	var sequence uint32
	var continued []byte
	packetSize := 0
	rate, channels, packets := 0, 0, 0
	ended := false
	for len(data) > 0 {
		if ended || len(data) < 27 || string(data[:4]) != "OggS" || data[4] != 0 {
			return errors.New("audio: invalid Ogg Vorbis: invalid page")
		}
		segments := int(data[26])
		if len(data) < 27+segments {
			return errors.New("audio: invalid Ogg Vorbis: truncated page")
		}
		pageSerial, pageSequence := binary.LittleEndian.Uint32(data[14:]), binary.LittleEndian.Uint32(data[18:])
		if sequence == 0 {
			serial = pageSerial
			if data[5]&2 == 0 {
				return errors.New("audio: invalid Ogg Vorbis: missing start")
			}
		} else if data[5]&2 != 0 {
			return errors.New("audio: invalid Ogg Vorbis: chained streams unsupported")
		}
		if pageSerial != serial || pageSequence != sequence || (data[5]&1 != 0) != (packetSize != 0) {
			return errors.New("audio: invalid Ogg Vorbis: discontinuous stream")
		}
		sequence++
		offset := 27 + segments
		packetStart := offset
		for _, length := range data[27:offset] {
			end := offset + int(length)
			if end > len(data) || packetSize+int(length) > 1<<20 {
				return errors.New("audio: invalid Ogg Vorbis: packet exceeds bounds")
			}
			packetSize += int(length)
			offset = end
			if length < 255 {
				if packets < 3 {
					packet := data[packetStart:offset]
					if len(continued) != 0 {
						continued = append(continued, packet...)
						packet = continued
					}
					switch packets {
					case 0:
						if len(packet) != 30 || !bytes.Equal(packet[:7], []byte("\x01vorbis")) || binary.LittleEndian.Uint32(packet[7:]) != 0 {
							return errors.New("audio: unsupported format (Ogg Vorbis required; Opus is unsupported)")
						}
						rate, channels = int(binary.LittleEndian.Uint32(packet[12:])), int(packet[11])
						if err := validLayout(rate, channels); err != nil {
							return err
						}
						small, large := packet[28]&15, packet[28]>>4
						if small < 6 || large > 13 || small > large || packet[29]&1 == 0 {
							return errors.New("audio: invalid Vorbis block sizes or framing")
						}
					case 1:
						if !boundedVorbisComments(packet) {
							return errors.New("audio: invalid or excessive Vorbis comments")
						}
					case 2:
						if !boundedVorbisSetup(packet, channels) {
							return errors.New("audio: invalid or excessive Vorbis setup")
						}
					}
					continued = continued[:0]
				}
				packets++
				packetSize = 0
				packetStart = offset
			}
		}
		if packets < 3 && packetSize != 0 {
			continued = append(continued, data[packetStart:offset]...)
		}
		granule := binary.LittleEndian.Uint64(data[6:])
		if rate != 0 && granule != ^uint64(0) && granule > uint64(rate)*maxAudioSeconds {
			return errors.New("audio: decoded PCM exceeds 120 seconds")
		}
		ended = data[5]&4 != 0
		data = data[offset:]
	}
	if !ended || packetSize != 0 || packets < 4 {
		return errors.New("audio: invalid Ogg Vorbis: incomplete stream")
	}
	return nil
}
