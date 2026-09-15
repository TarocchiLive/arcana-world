// SPDX-License-Identifier: GPL-3.0-only
// Wire format reference: https://github.com/xfgryujk/blivedm/blob/dev/blivedm/clients/ws_base.py (MIT).
package bili

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"

	"github.com/andybalholm/brotli"
)

const (
	danmakuMaxPacket   = 8 << 20
	danmakuMaxExpanded = 32 << 20
	danmakuMaxDepth    = 8
)

func danmakuPacket(op uint32, version uint16, body []byte) []byte {
	data := make([]byte, 16+len(body))
	binary.BigEndian.PutUint32(data, uint32(len(data)))
	binary.BigEndian.PutUint16(data[4:], 16)
	binary.BigEndian.PutUint16(data[6:], version)
	binary.BigEndian.PutUint32(data[8:], op)
	binary.BigEndian.PutUint32(data[12:], 1)
	copy(data[16:], body)
	return data
}

// The budget is cumulative across siblings and nesting, not a per-stream cap.
// Delivery is incremental so a corrupt suffix cannot discard valid predecessors.
func danmakuDecode(data []byte, depth int, budget *int, receive func(uint32, []byte) error) error {
	if depth > danmakuMaxDepth {
		return errors.New("danmaku: packet nesting limit exceeded")
	}
	for len(data) > 0 {
		if len(data) < 16 {
			return errors.New("danmaku: truncated packet header")
		}
		size := uint64(binary.BigEndian.Uint32(data))
		header := uint64(binary.BigEndian.Uint16(data[4:]))
		version := binary.BigEndian.Uint16(data[6:])
		op := binary.BigEndian.Uint32(data[8:])
		if header < 16 || size < header || size > uint64(len(data)) || size > danmakuMaxPacket {
			return errors.New("danmaku: invalid packet bounds")
		}
		body := data[header:size]
		switch version {
		case 0, 1:
			if err := receive(op, body); err != nil {
				return err
			}
		case 2, 3:
			if depth >= danmakuMaxDepth || *budget <= 0 {
				return errors.New("danmaku: packet expansion limit exceeded")
			}
			var reader io.Reader
			var closer io.Closer
			if version == 2 {
				zr, err := zlib.NewReader(bytes.NewReader(body))
				if err != nil {
					return errors.New("danmaku: invalid compressed packet")
				}
				reader = zr
				closer = zr
			} else {
				reader = brotli.NewReader(bytes.NewReader(body))
			}
			expanded, readErr := io.ReadAll(io.LimitReader(reader, int64(*budget)+1))
			if closer != nil {
				closer.Close()
			}
			overflow := len(expanded) > *budget
			if overflow {
				expanded = expanded[:*budget]
			}
			*budget -= len(expanded)
			if err := danmakuDecode(expanded, depth+1, budget, receive); err != nil {
				return err
			}
			if overflow {
				return errors.New("danmaku: packet expansion limit exceeded")
			}
			if readErr != nil {
				return errors.New("danmaku: invalid compressed packet")
			}
		default:
			return errors.New("danmaku: unsupported packet version")
		}
		data = data[size:]
	}
	return nil
}
